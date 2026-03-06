package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"codex-auth-refresher/internal/metrics"
	"codex-auth-refresher/internal/scheduler"
)

type fakeStatusSource struct {
	ready    bool
	snapshot scheduler.Snapshot
}

func (f fakeStatusSource) Ready() bool {
	return f.ready
}

func (f fakeStatusSource) Snapshot() scheduler.Snapshot {
	return f.snapshot
}

func TestHealthAndReadyEndpoints(t *testing.T) {
	t.Parallel()
	handler := NewHandler(fakeStatusSource{ready: false}, metrics.New(), Options{})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("healthz status = %d, want 200", resp.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/readyz", nil)
	resp = httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("readyz status = %d, want 503", resp.Code)
	}
}

func TestStatusEndpointRespectsFlag(t *testing.T) {
	t.Parallel()
	handler := NewHandler(fakeStatusSource{ready: true, snapshot: scheduler.Snapshot{StartedAt: time.Now().UTC()}}, metrics.New(), Options{StatusEnabled: false})

	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("status code = %d, want 404", resp.Code)
	}
}

func TestDashboardDisabledReturnsNotFound(t *testing.T) {
	t.Parallel()
	handler := NewHandler(fakeStatusSource{}, metrics.New(), Options{WebEnabled: false})

	for _, path := range []string{"/", "/v1/dashboard"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, req)
		if resp.Code != http.StatusNotFound {
			t.Fatalf("path %s code = %d, want 404", path, resp.Code)
		}
	}
}

func TestDashboardRootReturnsServiceUnavailableWhenAssetMissing(t *testing.T) {
	t.Parallel()
	oldLoader := uiIndexLoader
	uiIndexLoader = func() ([]byte, error) { return nil, errors.New("missing") }
	defer func() { uiIndexLoader = oldLoader }()

	handler := NewHandler(fakeStatusSource{}, metrics.New(), Options{WebEnabled: true})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("root code = %d, want 503", resp.Code)
	}
	if !strings.Contains(resp.Body.String(), "web UI is not built") {
		t.Fatalf("root body = %q, want missing build message", resp.Body.String())
	}
}

func TestDashboardEndpointsWhenEnabled(t *testing.T) {
	t.Parallel()
	oldLoader := uiIndexLoader
	uiIndexLoader = func() ([]byte, error) { return []byte("<!doctype html><html><body>dashboard</body></html>"), nil }
	defer func() { uiIndexLoader = oldLoader }()

	startedAt := time.Date(2026, 3, 6, 10, 0, 0, 0, time.UTC)
	lastScan := time.Date(2026, 3, 6, 11, 59, 30, 0, time.UTC)
	registry := metrics.New()
	registry.IncScans()
	registry.IncRefreshAttempts()
	registry.IncRefreshSuccess()
	registry.SetTrackedFiles(2, 1, 0)
	registrySnapshot := registry.Snapshot()
	if registrySnapshot.LastScanAt == nil {
		t.Fatal("expected LastScanAt to be set")
	}
	*registrySnapshot.LastScanAt = lastScan
	registry.IncRefreshAttempts()
	registry.IncRefreshFailure()

	handler := NewHandler(fakeStatusSource{
		ready: true,
		snapshot: scheduler.Snapshot{
			StartedAt: startedAt,
			AuthDir:   "/secret/auth",
			Files: []scheduler.FileStatus{
				{File: "b.json", State: "ok", Schema: "flat", ConsecutiveFailures: 0},
				{File: "a.json", AccountID: "acct-1", State: "reauth_required", LastError: "invalid grant", Disabled: true, ConsecutiveFailures: 3},
			},
		},
	}, registry, Options{
		StatusEnabled: true,
		WebEnabled:    true,
		RefreshBefore: "6h",
		RefreshMaxAge: "20h",
		ScanInterval:  "5m",
		MaxParallel:   1,
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("root code = %d, want 200", resp.Code)
	}
	if got := resp.Header().Get("Content-Security-Policy"); got == "" {
		t.Fatal("expected Content-Security-Policy header")
	}
	if got := resp.Header().Get("Content-Type"); !strings.Contains(got, "text/html") {
		t.Fatalf("root Content-Type = %q, want text/html", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/dashboard", nil)
	resp = httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("dashboard code = %d, want 200", resp.Code)
	}
	if got := resp.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("dashboard Cache-Control = %q, want no-store", got)
	}
	if got := resp.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("dashboard Content-Type = %q, want application/json", got)
	}

	var payload map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if _, ok := payload["auth_dir"]; ok {
		t.Fatal("dashboard payload must not expose auth_dir")
	}
	summary := payload["summary"].(map[string]any)
	if summary["tracked_files"].(float64) != 2 {
		t.Fatalf("tracked_files = %v, want 2", summary["tracked_files"])
	}
	if summary["reauth_required_files"].(float64) != 1 {
		t.Fatalf("reauth_required_files = %v, want 1", summary["reauth_required_files"])
	}
	config := payload["config"].(map[string]any)
	if config["refresh_max_age"].(string) != "20h" {
		t.Fatalf("refresh_max_age = %v, want 20h", config["refresh_max_age"])
	}
	files := payload["files"].([]any)
	if files[0].(map[string]any)["file"].(string) != "a.json" {
		t.Fatalf("first file = %v, want a.json sorted by priority", files[0].(map[string]any)["file"])
	}
}
