package refresher

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"codex-auth-refresher/internal/oauth"
)

type fakeTokenRefresher struct {
	response *oauth.Response
	err      error
}

func (f fakeTokenRefresher) Refresh(context.Context, string, string) (*oauth.Response, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.response, nil
}

func TestRefreshFileUpdatesTokensInPlace(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "user.json")
	input := []byte(`{
  "access_token": "` + testJWT(time.Now().Add(10*time.Minute), "client-1") + `",
  "refresh_token": "rt-old",
  "id_token": "` + testJWT(time.Now().Add(10*time.Minute), "") + `",
  "expired": "` + time.Now().Add(10*time.Minute).UTC().Format(time.RFC3339) + `",
  "last_refresh": "` + time.Now().Add(-time.Hour).UTC().Format(time.RFC3339) + `",
  "account_id": "acct-1"
}`)
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	service := NewService(fakeTokenRefresher{response: &oauth.Response{
		AccessToken:  testJWT(time.Now().Add(24*time.Hour), "client-1"),
		RefreshToken: "rt-new",
		IDToken:      testJWT(time.Now().Add(24*time.Hour), ""),
	}}, 6*time.Hour, 0, "fallback-client")
	result, err := service.RefreshFile(context.Background(), path)
	if err != nil {
		t.Fatalf("RefreshFile() error = %v", err)
	}
	if !result.Refreshed {
		t.Fatal("expected Refreshed=true")
	}
	output, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !json.Valid(output) {
		t.Fatalf("updated file is not valid JSON: %s", string(output))
	}
	var decoded map[string]any
	if err := json.Unmarshal(output, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if decoded["refresh_token"] != "rt-new" {
		t.Fatalf("refresh_token = %v, want rt-new", decoded["refresh_token"])
	}
	if decoded["id_token"] == "" {
		t.Fatal("expected id_token to be updated")
	}
}

func TestRefreshFileRejectsResponseWithoutExpiry(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "user.json")
	input := []byte(`{"access_token":"` + testJWT(time.Now().Add(time.Hour), "client-1") + `","refresh_token":"rt-old","id_token":"` + testJWT(time.Now().Add(time.Hour), "") + `"}`)
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	original, _ := os.ReadFile(path)

	service := NewService(fakeTokenRefresher{response: &oauth.Response{AccessToken: "opaque-access-token", IDToken: "opaque-id-token"}}, 2*time.Hour, 0, "fallback-client")
	_, err := service.RefreshFile(context.Background(), path)
	if !errors.Is(err, ErrUnknownExpiry) {
		t.Fatalf("RefreshFile() error = %v, want ErrUnknownExpiry", err)
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(original) {
		t.Fatalf("file was modified despite unknown expiry: before=%s after=%s", string(original), string(after))
	}
}

func TestRefreshFileReturnsInvalidGrantState(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "user.json")
	input := []byte(`{"access_token":"` + testJWT(time.Now().Add(time.Hour), "client-1") + `","refresh_token":"rt-old"}`)
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	service := NewService(fakeTokenRefresher{err: &oauth.Error{Code: "invalid_grant", Description: "refresh token already used"}}, 2*time.Hour, 0, "fallback-client")
	_, err := service.RefreshFile(context.Background(), path)
	if err == nil {
		t.Fatal("expected error")
	}
	var oauthErr *oauth.Error
	if !errors.As(err, &oauthErr) || !oauthErr.InvalidGrant() {
		t.Fatalf("expected invalid_grant error, got %v", err)
	}
}

func testJWT(exp time.Time, clientID string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payloadMap := map[string]any{"exp": exp.Unix()}
	if clientID != "" {
		payloadMap["client_id"] = clientID
	}
	payload, _ := json.Marshal(payloadMap)
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	return header + "." + encodedPayload + ".sig"
}

func TestInspectFileRefreshesWhenMaxAgeReached(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 6, 15, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	path := filepath.Join(dir, "user.json")
	lastRefresh := now.Add(-25 * time.Hour)
	accessExpiry := now.Add(10 * 24 * time.Hour)
	input := []byte(`{
  "access_token": "` + testJWT(accessExpiry, "client-1") + `",
  "refresh_token": "rt-old",
  "expired": "` + accessExpiry.UTC().Format(time.RFC3339) + `",
  "last_refresh": "` + lastRefresh.UTC().Format(time.RFC3339) + `"
}`)
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	service := NewService(fakeTokenRefresher{}, 6*time.Hour, 24*time.Hour, "fallback-client")
	service.now = func() time.Time { return now }

	inspection, err := service.InspectFile(path)
	if err != nil {
		t.Fatalf("InspectFile() error = %v", err)
	}
	if !inspection.RefreshDue {
		t.Fatal("expected RefreshDue=true when refresh-max-age is exceeded")
	}
	wantNext := lastRefresh.Add(24 * time.Hour)
	if inspection.NextRefreshAt == nil || !inspection.NextRefreshAt.Equal(wantNext) {
		t.Fatalf("NextRefreshAt = %v, want %v", inspection.NextRefreshAt, wantNext)
	}
}

func TestInspectFileUsesEarlierOfExpiryAndMaxAge(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 6, 15, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	path := filepath.Join(dir, "user.json")
	lastRefresh := now.Add(-2 * time.Hour)
	accessExpiry := now.Add(72 * time.Hour)
	input := []byte(`{
  "access_token": "` + testJWT(accessExpiry, "client-1") + `",
  "refresh_token": "rt-old",
  "expired": "` + accessExpiry.UTC().Format(time.RFC3339) + `",
  "last_refresh": "` + lastRefresh.UTC().Format(time.RFC3339) + `"
}`)
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	service := NewService(fakeTokenRefresher{}, 6*time.Hour, 24*time.Hour, "fallback-client")
	service.now = func() time.Time { return now }

	inspection, err := service.InspectFile(path)
	if err != nil {
		t.Fatalf("InspectFile() error = %v", err)
	}
	if inspection.RefreshDue {
		t.Fatal("expected RefreshDue=false while both schedules are still in the future")
	}
	wantNext := lastRefresh.Add(24 * time.Hour)
	if inspection.NextRefreshAt == nil || !inspection.NextRefreshAt.Equal(wantNext) {
		t.Fatalf("NextRefreshAt = %v, want %v", inspection.NextRefreshAt, wantNext)
	}
}

func TestInspectFileRefreshesImmediatelyWithoutLastRefreshWhenMaxAgeEnabled(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 6, 15, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	path := filepath.Join(dir, "user.json")
	accessExpiry := now.Add(10 * 24 * time.Hour)
	input := []byte(`{
  "access_token": "` + testJWT(accessExpiry, "client-1") + `",
  "refresh_token": "rt-old",
  "expired": "` + accessExpiry.UTC().Format(time.RFC3339) + `"
}`)
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	service := NewService(fakeTokenRefresher{}, 6*time.Hour, 24*time.Hour, "fallback-client")
	service.now = func() time.Time { return now }

	inspection, err := service.InspectFile(path)
	if err != nil {
		t.Fatalf("InspectFile() error = %v", err)
	}
	if !inspection.RefreshDue {
		t.Fatal("expected RefreshDue=true when refresh-max-age is enabled but last_refresh is missing")
	}
	if inspection.NextRefreshAt == nil || !inspection.NextRefreshAt.Equal(now) {
		t.Fatalf("NextRefreshAt = %v, want %v", inspection.NextRefreshAt, now)
	}
}
