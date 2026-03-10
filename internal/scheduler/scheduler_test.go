package scheduler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"codex-auth-refresher/internal/alerting"
	"codex-auth-refresher/internal/metrics"
	"codex-auth-refresher/internal/oauth"
	"codex-auth-refresher/internal/refresher"
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

type blockingTokenRefresher struct {
	response      *oauth.Response
	release       chan struct{}
	mu            sync.Mutex
	calls         int
	inFlight      int
	maxInFlight   int
	clientIDsSeen []string
}

func (b *blockingTokenRefresher) Refresh(_ context.Context, _ string, clientID string) (*oauth.Response, error) {
	b.mu.Lock()
	b.calls++
	b.inFlight++
	if b.inFlight > b.maxInFlight {
		b.maxInFlight = b.inFlight
	}
	b.clientIDsSeen = append(b.clientIDsSeen, clientID)
	b.mu.Unlock()

	<-b.release

	b.mu.Lock()
	b.inFlight--
	b.mu.Unlock()
	return b.response, nil
}

func (b *blockingTokenRefresher) Stats() (calls int, inFlight int, maxInFlight int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.calls, b.inFlight, b.maxInFlight
}

type fakeNotifier struct {
	mu        sync.Mutex
	snapshots []alerting.Snapshot
}

func (f *fakeNotifier) Notify(snapshot alerting.Snapshot) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.snapshots = append(f.snapshots, snapshot)
}

func (f *fakeNotifier) Snapshots() []alerting.Snapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]alerting.Snapshot, len(f.snapshots))
	copy(out, f.snapshots)
	return out
}

func TestManagerRefreshesValidFilesAndKeepsInvalidJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	validPath := filepath.Join(dir, "codex-valid.json")
	invalidPath := filepath.Join(dir, "codex-broken.json")
	soon := time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339)
	if err := os.WriteFile(validPath, []byte(`{"access_token":"`+testJWT(time.Now().Add(10*time.Minute), "client-1")+`","refresh_token":"rt-1","expired":"`+soon+`"}`), 0o600); err != nil {
		t.Fatalf("WriteFile(valid) error = %v", err)
	}
	if err := os.WriteFile(invalidPath, []byte(`{"access_token":`), 0o600); err != nil {
		t.Fatalf("WriteFile(invalid) error = %v", err)
	}

	refreshService := refresher.NewService(fakeTokenRefresher{response: &oauth.Response{AccessToken: testJWT(time.Now().Add(24*time.Hour), "client-1")}}, 6*time.Hour, 0, "fallback-client")
	manager := NewManager(dir, 50*time.Millisecond, 1, refreshService, metrics.New(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	manager.watchFactory = nil

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = manager.Run(ctx) }()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snapshot := manager.Snapshot()
		if len(snapshot.Files) == 2 {
			states := map[string]string{}
			for _, file := range snapshot.Files {
				states[file.File] = string(file.State)
			}
			if states["codex-valid.json"] == "ok" && states["codex-broken.json"] == "invalid_json" {
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("snapshot did not reach expected state: %+v", manager.Snapshot())
}

func TestManagerAppliesBackoffOnTooManyRequests(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "codex-rate-limited.json")
	soon := time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339)
	if err := os.WriteFile(path, []byte(`{"access_token":"`+testJWT(time.Now().Add(10*time.Minute), "client-1")+`","refresh_token":"rt-1","expired":"`+soon+`"}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	refreshService := refresher.NewService(fakeTokenRefresher{err: &oauth.Error{StatusCode: 429, Code: "rate_limited", Description: "too many requests", Retryable: true}}, 6*time.Hour, 0, "fallback-client")
	manager := NewManager(dir, time.Hour, 1, refreshService, metrics.New(), slog.New(slog.NewTextHandler(io.Discard, nil)))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go manager.worker(ctx)
	before := time.Now().UTC()
	if err := manager.scanOnce(context.Background()); err != nil {
		t.Fatalf("scanOnce() error = %v", err)
	}

	var retryAt time.Time
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		manager.mu.RLock()
		record := manager.files[path]
		manager.mu.RUnlock()
		if record != nil && record.status.State == refresher.StateDegraded && !record.nextAttemptAt.IsZero() {
			minExpected := before.Add(50 * time.Second)
			maxExpected := before.Add(70 * time.Second)
			if record.nextAttemptAt.Before(minExpected) || record.nextAttemptAt.After(maxExpected) {
				t.Fatalf("nextAttemptAt = %v, want between %v and %v", record.nextAttemptAt, minExpected, maxExpected)
			}
			if record.status.ConsecutiveFailures != 1 {
				t.Fatalf("ConsecutiveFailures = %d, want 1", record.status.ConsecutiveFailures)
			}
			if record.status.NextRefreshAt == nil || !record.status.NextRefreshAt.Equal(record.nextAttemptAt) {
				t.Fatalf("status.NextRefreshAt = %v, want %v", record.status.NextRefreshAt, record.nextAttemptAt)
			}
			retryAt = record.nextAttemptAt
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if retryAt.IsZero() {
		t.Fatalf("expected degraded record with backoff, got %+v", manager.Snapshot())
	}

	if err := manager.scanOnce(context.Background()); err != nil {
		t.Fatalf("scanOnce() second error = %v", err)
	}

	manager.mu.RLock()
	record := manager.files[path]
	manager.mu.RUnlock()
	if record == nil {
		t.Fatal("expected tracked file after second scan")
	}
	if record.status.State != refresher.StateDegraded {
		t.Fatalf("state after second scan = %q, want %q", record.status.State, refresher.StateDegraded)
	}
	if record.status.NextRefreshAt == nil || !record.status.NextRefreshAt.Equal(retryAt) {
		t.Fatalf("status.NextRefreshAt after second scan = %v, want %v", record.status.NextRefreshAt, retryAt)
	}
}

func TestManagerSerializesSameAccountRefreshes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	soon := time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339)
	for _, name := range []string{"codex-a.json", "codex-b.json"} {
		content := `{"access_token":"` + testJWT(time.Now().Add(10*time.Minute), "client-1") + `","refresh_token":"rt-1","expired":"` + soon + `","account_id":"acct-1"}`
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", name, err)
		}
	}

	blocking := &blockingTokenRefresher{response: &oauth.Response{AccessToken: testJWT(time.Now().Add(24*time.Hour), "client-1")}, release: make(chan struct{})}
	refreshService := refresher.NewService(blocking, 6*time.Hour, 0, "fallback-client")
	manager := NewManager(dir, time.Hour, 2, refreshService, metrics.New(), slog.New(slog.NewTextHandler(io.Discard, nil)))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go manager.worker(ctx)
	go manager.worker(ctx)

	if err := manager.scanOnce(context.Background()); err != nil {
		t.Fatalf("scanOnce() error = %v", err)
	}
	waitUntil(t, 2*time.Second, func() bool {
		calls, _, _ := blocking.Stats()
		return calls == 1
	})

	if err := manager.scanOnce(context.Background()); err != nil {
		t.Fatalf("scanOnce() second error = %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	if calls, _, maxInFlight := blocking.Stats(); calls != 1 || maxInFlight != 1 {
		t.Fatalf("after second scan while blocked: calls=%d maxInFlight=%d, want 1 and 1", calls, maxInFlight)
	}

	close(blocking.release)
	waitUntil(t, 2*time.Second, func() bool {
		_, inFlight, _ := blocking.Stats()
		return inFlight == 0
	})
	if err := manager.scanOnce(context.Background()); err != nil {
		t.Fatalf("scanOnce() third error = %v", err)
	}
	waitUntil(t, 2*time.Second, func() bool {
		calls, _, _ := blocking.Stats()
		return calls == 2
	})
	if _, _, maxInFlight := blocking.Stats(); maxInFlight != 1 {
		t.Fatalf("maxInFlight = %d, want 1", maxInFlight)
	}
}

func TestManagerPreservesReauthRequiredStatusAcrossScans(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "codex-reauth.json")
	soon := time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339)
	if err := os.WriteFile(path, []byte(`{"access_token":"`+testJWT(time.Now().Add(10*time.Minute), "client-1")+`","refresh_token":"rt-1","expired":"`+soon+`"}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	refreshService := refresher.NewService(fakeTokenRefresher{err: &oauth.Error{StatusCode: 400, Code: "invalid_grant", Description: "refresh token expired"}}, 6*time.Hour, 0, "fallback-client")
	manager := NewManager(dir, time.Hour, 1, refreshService, metrics.New(), slog.New(slog.NewTextHandler(io.Discard, nil)))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go manager.worker(ctx)

	if err := manager.scanOnce(context.Background()); err != nil {
		t.Fatalf("scanOnce() error = %v", err)
	}

	var retryAt time.Time
	waitUntil(t, 2*time.Second, func() bool {
		manager.mu.RLock()
		defer manager.mu.RUnlock()
		record := manager.files[path]
		if record == nil || record.status.State != refresher.StateReauthRequired || record.nextAttemptAt.IsZero() {
			return false
		}
		retryAt = record.nextAttemptAt
		return true
	})

	if err := manager.scanOnce(context.Background()); err != nil {
		t.Fatalf("scanOnce() second error = %v", err)
	}

	snapshot := manager.Snapshot()
	if len(snapshot.Files) != 1 {
		t.Fatalf("len(snapshot.Files) = %d, want 1", len(snapshot.Files))
	}
	if snapshot.Files[0].State != refresher.StateReauthRequired {
		t.Fatalf("state after second scan = %q, want %q", snapshot.Files[0].State, refresher.StateReauthRequired)
	}
	if snapshot.Files[0].NextRefreshAt == nil || !snapshot.Files[0].NextRefreshAt.Equal(retryAt) {
		t.Fatalf("status.NextRefreshAt after second scan = %v, want %v", snapshot.Files[0].NextRefreshAt, retryAt)
	}
}

func TestManagerIgnoresNonCodexFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	soon := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)

	if err := os.WriteFile(filepath.Join(dir, "codex-user.json"), []byte(`{
  "access_token":"`+testJWT(time.Now().Add(24*time.Hour), "client-1")+`",
  "refresh_token":"rt-1",
  "expired":"`+soon+`"
}`), 0o600); err != nil {
		t.Fatalf("WriteFile(codex-user) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "claude-user.json"), []byte(`{
  "access_token":"`+testJWT(time.Now().Add(24*time.Hour), "client-1")+`",
  "refresh_token":"rt-2",
  "expired":"`+soon+`"
}`), 0o600); err != nil {
		t.Fatalf("WriteFile(claude-user) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "gemini-broken.json"), []byte(`{"access_token":`), 0o600); err != nil {
		t.Fatalf("WriteFile(gemini-broken) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "codex-foreign.json"), []byte(`{
  "access_token":"`+testJWT(time.Now().Add(24*time.Hour), "client-1")+`",
  "refresh_token":"rt-3",
  "expired":"`+soon+`",
  "type":"claude"
}`), 0o600); err != nil {
		t.Fatalf("WriteFile(codex-foreign) error = %v", err)
	}

	refreshService := refresher.NewService(fakeTokenRefresher{}, 6*time.Hour, 0, "fallback-client")
	manager := NewManager(dir, time.Hour, 1, refreshService, metrics.New(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	manager.watchFactory = nil

	if err := manager.scanOnce(context.Background()); err != nil {
		t.Fatalf("scanOnce() error = %v", err)
	}

	snapshot := manager.Snapshot()
	if len(snapshot.Files) != 1 {
		t.Fatalf("len(snapshot.Files) = %d, want 1; snapshot=%+v", len(snapshot.Files), snapshot)
	}
	if got := snapshot.Files[0].File; got != "codex-user.json" {
		t.Fatalf("snapshot.Files[0].File = %q, want codex-user.json", got)
	}
}

func TestManagerPublishesProblemSnapshotsForAlerts(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	validPath := filepath.Join(dir, "codex-valid.json")
	invalidPath := filepath.Join(dir, "codex-broken.json")
	soon := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)

	if err := os.WriteFile(validPath, []byte(`{"access_token":"`+testJWT(time.Now().Add(24*time.Hour), "client-1")+`","refresh_token":"rt-1","expired":"`+soon+`"}`), 0o600); err != nil {
		t.Fatalf("WriteFile(valid) error = %v", err)
	}
	if err := os.WriteFile(invalidPath, []byte(`{"access_token":`), 0o600); err != nil {
		t.Fatalf("WriteFile(invalid) error = %v", err)
	}

	refreshService := refresher.NewService(fakeTokenRefresher{}, 6*time.Hour, 0, "fallback-client")
	manager := NewManager(dir, time.Hour, 1, refreshService, metrics.New(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	notifier := &fakeNotifier{}
	manager.SetNotifier(notifier)

	if err := manager.scanOnce(context.Background()); err != nil {
		t.Fatalf("scanOnce() error = %v", err)
	}

	snapshots := notifier.Snapshots()
	if len(snapshots) != 1 {
		t.Fatalf("len(snapshots) = %d, want 1", len(snapshots))
	}
	if len(snapshots[0].Problems) != 1 {
		t.Fatalf("len(snapshot.Problems) = %d, want 1", len(snapshots[0].Problems))
	}
	problem := snapshots[0].Problems[0]
	if problem.Path != invalidPath {
		t.Fatalf("problem.Path = %q, want %q", problem.Path, invalidPath)
	}
	if problem.State != string(refresher.StateInvalidJSON) {
		t.Fatalf("problem.State = %q, want %q", problem.State, refresher.StateInvalidJSON)
	}

	if err := os.WriteFile(invalidPath, []byte(`{"access_token":"`+testJWT(time.Now().Add(24*time.Hour), "client-1")+`","refresh_token":"rt-2","expired":"`+soon+`"}`), 0o600); err != nil {
		t.Fatalf("WriteFile(fixed) error = %v", err)
	}
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(invalidPath, future, future); err != nil {
		t.Fatalf("Chtimes() error = %v", err)
	}
	if err := manager.scanOnce(context.Background()); err != nil {
		t.Fatalf("scanOnce() second error = %v", err)
	}

	snapshots = notifier.Snapshots()
	if len(snapshots) != 2 {
		t.Fatalf("len(snapshots) = %d, want 2", len(snapshots))
	}
	if len(snapshots[1].Problems) != 0 {
		t.Fatalf("len(second snapshot problems) = %d, want 0", len(snapshots[1].Problems))
	}
}

func waitUntil(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
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
