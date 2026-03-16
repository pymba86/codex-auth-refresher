package refresher

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"codex-auth-refresher/internal/oauth"
)

type fakeTokenRefresher struct {
	mu       sync.Mutex
	response *oauth.Response
	err      error
	requests []oauth.Request
}

func (f *fakeTokenRefresher) Refresh(_ context.Context, request oauth.Request) (*oauth.Response, error) {
	f.mu.Lock()
	f.requests = append(f.requests, request)
	f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	return f.response, nil
}

func (f *fakeTokenRefresher) LastRequest() oauth.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.requests) == 0 {
		return oauth.Request{}
	}
	return f.requests[len(f.requests)-1]
}

func testProviderConfig() ProviderConfig {
	return ProviderConfig{
		CodexTokenEndpoint:       "https://auth.openai.com/oauth/token",
		CodexClientID:            "fallback-client",
		AntigravityTokenEndpoint: "https://oauth2.googleapis.com/token",
		AntigravityClientID:      "antigravity-client-id",
		AntigravityClientSecret:  "antigravity-client-secret",
	}
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

	refresherClient := &fakeTokenRefresher{response: &oauth.Response{
		AccessToken:  testJWT(time.Now().Add(24*time.Hour), "client-1"),
		RefreshToken: "rt-new",
		IDToken:      testJWT(time.Now().Add(24*time.Hour), ""),
	}}
	service := NewService(refresherClient, 6*time.Hour, 0, testProviderConfig())
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
	if got := refresherClient.LastRequest().Endpoint; got != "https://auth.openai.com/oauth/token" {
		t.Fatalf("request endpoint = %q, want codex token endpoint", got)
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

	service := NewService(&fakeTokenRefresher{response: &oauth.Response{AccessToken: "opaque-access-token", IDToken: "opaque-id-token"}}, 2*time.Hour, 0, testProviderConfig())
	_, err := service.RefreshFile(context.Background(), path)
	if !errors.Is(err, ErrUnknownExpiry) {
		t.Fatalf("RefreshFile() error = %v, want ErrUnknownExpiry", err)
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(original) {
		t.Fatalf("file was modified despite unknown expiry: before=%s after=%s", string(original), string(after))
	}
}

func TestInspectFileRejectsNonCodexType(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "codex-user.json")
	input := []byte(`{
  "access_token":"` + testJWT(time.Now().Add(time.Hour), "client-1") + `",
  "refresh_token":"rt-old",
  "type":"claude"
}`)
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	service := NewService(&fakeTokenRefresher{}, 2*time.Hour, 0, testProviderConfig())
	_, err := service.InspectFile(path)
	if !errors.Is(err, ErrUnsupportedAuth) {
		t.Fatalf("InspectFile() error = %v, want ErrUnsupportedAuth", err)
	}
}

func TestRefreshFileRejectsNonCodexType(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "codex-user.json")
	input := []byte(`{
  "access_token":"` + testJWT(time.Now().Add(time.Hour), "client-1") + `",
  "refresh_token":"rt-old",
  "type":"claude"
}`)
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	service := NewService(&fakeTokenRefresher{}, 2*time.Hour, 0, testProviderConfig())
	_, err := service.RefreshFile(context.Background(), path)
	if !errors.Is(err, ErrUnsupportedAuth) {
		t.Fatalf("RefreshFile() error = %v, want ErrUnsupportedAuth", err)
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

	service := NewService(&fakeTokenRefresher{err: &oauth.Error{Code: "invalid_grant", Description: "refresh token already used"}}, 2*time.Hour, 0, testProviderConfig())
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

func TestRefreshFileGeminiUsesNestedOAuthMetadata(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 16, 10, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	path := filepath.Join(dir, "gemini-user.json")
	input := []byte(`{
  "type": "gemini",
  "timestamp": 1700000000000,
  "token": {
    "access_token": "opaque-old",
    "refresh_token": "rt-old",
    "client_id": "gemini-client",
    "client_secret": "gemini-secret",
    "token_uri": "https://oauth2.googleapis.com/token",
    "expires_in": 3599,
    "expiry": "2026-03-16T14:05:18.621567873+05:00"
  }
}`)
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	client := &fakeTokenRefresher{response: &oauth.Response{
		AccessToken:  "opaque-new",
		RefreshToken: "rt-new",
		ExpiresIn:    7200,
	}}
	service := NewService(client, 6*time.Hour, 0, testProviderConfig())
	service.now = func() time.Time { return now }

	result, err := service.RefreshFile(context.Background(), path)
	if err != nil {
		t.Fatalf("RefreshFile() error = %v", err)
	}
	if !result.Refreshed {
		t.Fatal("expected Refreshed=true")
	}
	request := client.LastRequest()
	if request.Endpoint != "https://oauth2.googleapis.com/token" {
		t.Fatalf("request.Endpoint = %q, want Google token endpoint", request.Endpoint)
	}
	if request.ClientID != "gemini-client" {
		t.Fatalf("request.ClientID = %q, want gemini-client", request.ClientID)
	}
	if request.ClientSecret != "gemini-secret" {
		t.Fatalf("request.ClientSecret = %q, want gemini-secret", request.ClientSecret)
	}

	output, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(output, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if _, ok := decoded["token"]; !ok {
		t.Fatal("expected nested token container to remain token")
	}
	var token map[string]any
	if err := json.Unmarshal(decoded["token"], &token); err != nil {
		t.Fatalf("json.Unmarshal(token) error = %v", err)
	}
	if got := token["access_token"]; got != "opaque-new" {
		t.Fatalf("token.access_token = %v, want opaque-new", got)
	}
	if got := token["refresh_token"]; got != "rt-new" {
		t.Fatalf("token.refresh_token = %v, want rt-new", got)
	}
	if got := token["expires_in"]; got != float64(7200) {
		t.Fatalf("token.expires_in = %v, want 7200", got)
	}
}

func TestRefreshFileAntigravityUsesProviderDefaults(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 16, 10, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	path := filepath.Join(dir, "antigravity-user.json")
	input := []byte(`{
  "type": "antigravity",
  "access_token": "opaque-old",
  "refresh_token": "rt-old",
  "expires_in": 3599,
  "expired": "2026-03-16T15:10:21+05:00",
  "timestamp": 1773652222957
}`)
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	client := &fakeTokenRefresher{response: &oauth.Response{
		AccessToken:  "opaque-new",
		RefreshToken: "rt-new",
		ExpiresIn:    1800,
	}}
	service := NewService(client, 6*time.Hour, 0, testProviderConfig())
	service.now = func() time.Time { return now }

	if _, err := service.RefreshFile(context.Background(), path); err != nil {
		t.Fatalf("RefreshFile() error = %v", err)
	}
	request := client.LastRequest()
	if request.Endpoint != testProviderConfig().AntigravityTokenEndpoint {
		t.Fatalf("request.Endpoint = %q, want antigravity default endpoint", request.Endpoint)
	}
	if request.ClientID != testProviderConfig().AntigravityClientID {
		t.Fatalf("request.ClientID = %q, want antigravity default client id", request.ClientID)
	}
	if request.ClientSecret != testProviderConfig().AntigravityClientSecret {
		t.Fatalf("request.ClientSecret = %q, want antigravity default client secret", request.ClientSecret)
	}

	output, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(output, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got := decoded["access_token"]; got != "opaque-new" {
		t.Fatalf("access_token = %v, want opaque-new", got)
	}
	if got := decoded["refresh_token"]; got != "rt-new" {
		t.Fatalf("refresh_token = %v, want rt-new", got)
	}
	if got := decoded["expires_in"]; got != float64(1800) {
		t.Fatalf("expires_in = %v, want 1800", got)
	}
}

func TestRefreshFileAntigravityRequiresClientID(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "antigravity-user.json")
	input := []byte(`{
  "type": "antigravity",
  "access_token": "opaque-old",
  "refresh_token": "rt-old",
  "expired": "2026-03-16T15:10:21+05:00"
}`)
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg := testProviderConfig()
	cfg.AntigravityClientID = ""
	service := NewService(&fakeTokenRefresher{}, 6*time.Hour, 0, cfg)

	_, err := service.RefreshFile(context.Background(), path)
	if !errors.Is(err, ErrMissingClientID) {
		t.Fatalf("RefreshFile() error = %v, want ErrMissingClientID", err)
	}
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

	service := NewService(&fakeTokenRefresher{}, 6*time.Hour, 24*time.Hour, testProviderConfig())
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

	service := NewService(&fakeTokenRefresher{}, 6*time.Hour, 24*time.Hour, testProviderConfig())
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

	service := NewService(&fakeTokenRefresher{}, 6*time.Hour, 24*time.Hour, testProviderConfig())
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

func TestInspectFileDoesNotImmediatelyRetryFreshShortLivedToken(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 16, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	path := filepath.Join(dir, "gemini-user.json")
	lastRefresh := now.Add(-2 * time.Minute)
	accessExpiry := now.Add(time.Hour)
	input := []byte(`{
  "type": "gemini",
  "token": {
    "access_token": "` + testJWT(accessExpiry, "gemini-client") + `",
    "refresh_token": "rt-old",
    "client_id": "gemini-client",
    "client_secret": "gemini-secret",
    "token_uri": "https://oauth2.googleapis.com/token",
    "expiry": "` + accessExpiry.UTC().Format(time.RFC3339) + `"
  },
  "last_refresh": "` + lastRefresh.UTC().Format(time.RFC3339) + `"
}`)
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	service := NewService(&fakeTokenRefresher{}, 6*time.Hour, 0, testProviderConfig())
	service.now = func() time.Time { return now }

	inspection, err := service.InspectFile(path)
	if err != nil {
		t.Fatalf("InspectFile() error = %v", err)
	}
	if inspection.RefreshDue {
		t.Fatal("expected RefreshDue=false for a freshly refreshed short-lived token")
	}
	if inspection.NextRefreshAt == nil || !inspection.NextRefreshAt.Equal(accessExpiry) {
		t.Fatalf("NextRefreshAt = %v, want %v", inspection.NextRefreshAt, accessExpiry)
	}
}
