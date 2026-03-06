package httpapi

import (
	"encoding/json"
	"net/http"

	"codex-auth-refresher/internal/metrics"
	"codex-auth-refresher/internal/scheduler"
)

type statusSource interface {
	Ready() bool
	Snapshot() scheduler.Snapshot
}

func NewHandler(source statusSource, metricsRegistry *metrics.Registry, authDir string, statusEnabled bool) http.Handler {
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
		if !statusEnabled {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(source.Snapshot())
	})
	return mux
}
