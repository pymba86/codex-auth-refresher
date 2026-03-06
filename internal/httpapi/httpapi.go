package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"codex-auth-refresher/internal/metrics"
	"codex-auth-refresher/internal/scheduler"
)

type statusSource interface {
	Ready() bool
	Snapshot() scheduler.Snapshot
}

type Options struct {
	StatusEnabled bool
	WebEnabled    bool
	RefreshBefore string
	RefreshMaxAge string
	ScanInterval  string
	MaxParallel   int
}

type DashboardResponse struct {
	GeneratedAt time.Time        `json:"generated_at"`
	Service     DashboardService `json:"service"`
	Config      DashboardConfig  `json:"config"`
	Metrics     DashboardMetrics `json:"metrics"`
	Summary     DashboardSummary `json:"summary"`
	Files       []DashboardFile  `json:"files"`
}

type DashboardService struct {
	Ready         bool      `json:"ready"`
	StartedAt     time.Time `json:"started_at"`
	UptimeSeconds int64     `json:"uptime_seconds"`
}

type DashboardConfig struct {
	RefreshBefore    string `json:"refresh_before"`
	RefreshMaxAge    string `json:"refresh_max_age"`
	ScanInterval     string `json:"scan_interval"`
	MaxParallel      int    `json:"max_parallel"`
	StatusAPIEnabled bool   `json:"status_api_enabled"`
}

type DashboardMetrics struct {
	ScansTotal           uint64     `json:"scans_total"`
	RefreshAttemptsTotal uint64     `json:"refresh_attempts_total"`
	RefreshSuccessTotal  uint64     `json:"refresh_success_total"`
	RefreshFailureTotal  uint64     `json:"refresh_failure_total"`
	LastScanAt           *time.Time `json:"last_scan_at,omitempty"`
}

type DashboardSummary struct {
	TrackedFiles        int `json:"tracked_files"`
	OKFiles             int `json:"ok_files"`
	DegradedFiles       int `json:"degraded_files"`
	ReauthRequiredFiles int `json:"reauth_required_files"`
	InvalidJSONFiles    int `json:"invalid_json_files"`
	DisabledFiles       int `json:"disabled_files"`
}

type DashboardFile struct {
	File                string     `json:"file"`
	AccountID           string     `json:"account_id,omitempty"`
	Schema              string     `json:"schema,omitempty"`
	State               string     `json:"state"`
	ExpiresAt           *time.Time `json:"expires_at,omitempty"`
	NextRefreshAt       *time.Time `json:"next_refresh_at,omitempty"`
	LastRefreshAt       *time.Time `json:"last_refresh_at,omitempty"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	Disabled            bool       `json:"disabled"`
	LastError           string     `json:"last_error,omitempty"`
}

var uiIndexLoader = loadEmbeddedIndexHTML

func NewHandler(source statusSource, metricsRegistry *metrics.Registry, options Options) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if !source.Ready() {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ready\n"))
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte(metricsRegistry.RenderPrometheus()))
	})
	mux.HandleFunc("/v1/status", func(w http.ResponseWriter, r *http.Request) {
		if !options.StatusEnabled {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(source.Snapshot())
	})
	mux.HandleFunc("/v1/dashboard", func(w http.ResponseWriter, r *http.Request) {
		if !options.WebEnabled {
			http.NotFound(w, r)
			return
		}
		applyAPIHeaders(w)
		w.Header().Set("Content-Type", "application/json")
		response := buildDashboardResponse(source.Ready(), source.Snapshot(), metricsRegistry.Snapshot(), options, time.Now().UTC())
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(response)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if !options.WebEnabled {
			http.NotFound(w, r)
			return
		}
		indexHTML, err := uiIndexLoader()
		if err != nil {
			applyHTMLHeaders(w)
			http.Error(w, fmt.Sprintf("web UI is not built; run npm run build --prefix web (%v)", err), http.StatusServiceUnavailable)
			return
		}
		applyHTMLHeaders(w)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(indexHTML)
	})
	return mux
}

func buildDashboardResponse(ready bool, status scheduler.Snapshot, metricSnapshot metrics.Snapshot, options Options, now time.Time) DashboardResponse {
	files := make([]DashboardFile, 0, len(status.Files))
	summary := DashboardSummary{TrackedFiles: len(status.Files)}
	for _, file := range status.Files {
		if file.Disabled {
			summary.DisabledFiles++
		}
		switch string(file.State) {
		case "ok":
			summary.OKFiles++
		case "degraded":
			summary.DegradedFiles++
		case "reauth_required":
			summary.ReauthRequiredFiles++
		case "invalid_json":
			summary.InvalidJSONFiles++
		}
		files = append(files, DashboardFile{
			File:                file.File,
			AccountID:           file.AccountID,
			Schema:              file.Schema,
			State:               string(file.State),
			ExpiresAt:           cloneTime(file.ExpiresAt),
			NextRefreshAt:       cloneTime(file.NextRefreshAt),
			LastRefreshAt:       cloneTime(file.LastRefreshAt),
			ConsecutiveFailures: file.ConsecutiveFailures,
			Disabled:            file.Disabled,
			LastError:           file.LastError,
		})
	}
	sort.SliceStable(files, func(i, j int) bool {
		left := dashboardPriority(files[i].State)
		right := dashboardPriority(files[j].State)
		if left != right {
			return left < right
		}
		return strings.ToLower(files[i].File) < strings.ToLower(files[j].File)
	})

	uptime := now.Sub(status.StartedAt.UTC())
	if uptime < 0 {
		uptime = 0
	}

	return DashboardResponse{
		GeneratedAt: now,
		Service: DashboardService{
			Ready:         ready,
			StartedAt:     status.StartedAt.UTC(),
			UptimeSeconds: int64(uptime / time.Second),
		},
		Config: DashboardConfig{
			RefreshBefore:    options.RefreshBefore,
			RefreshMaxAge:    options.RefreshMaxAge,
			ScanInterval:     options.ScanInterval,
			MaxParallel:      options.MaxParallel,
			StatusAPIEnabled: options.StatusEnabled,
		},
		Metrics: DashboardMetrics{
			ScansTotal:           metricSnapshot.ScansTotal,
			RefreshAttemptsTotal: metricSnapshot.RefreshAttempts,
			RefreshSuccessTotal:  metricSnapshot.RefreshSuccessTotal,
			RefreshFailureTotal:  metricSnapshot.RefreshFailureTotal,
			LastScanAt:           cloneTime(metricSnapshot.LastScanAt),
		},
		Summary: summary,
		Files:   files,
	}
}

func applyHTMLHeaders(w http.ResponseWriter) {
	applyAPIHeaders(w)
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'unsafe-inline' 'self'; script-src 'unsafe-inline' 'self'; img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
}

func applyAPIHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

func dashboardPriority(state string) int {
	switch state {
	case "reauth_required":
		return 0
	case "invalid_json":
		return 1
	case "degraded":
		return 2
	case "ok":
		return 3
	default:
		return 4
	}
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := value.UTC()
	return &copy
}
