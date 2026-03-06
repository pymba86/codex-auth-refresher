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

func TestManagerRefreshesValidFilesAndKeepsInvalidJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	validPath := filepath.Join(dir, "valid.json")
	invalidPath := filepath.Join(dir, "broken.json")
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
			if states["valid.json"] == "ok" && states["broken.json"] == "invalid_json" {
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
	path := filepath.Join(dir, "rate-limited.json")
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
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("expected degraded record with backoff, got %+v", manager.Snapshot())
}

func TestManagerSerializesSameAccountRefreshes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	soon := time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339)
	for _, name := range []string{"a.json", "b.json"} {
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
