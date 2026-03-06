package metrics

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

type Snapshot struct {
	ScansTotal          uint64
	RefreshAttempts     uint64
	RefreshSuccessTotal uint64
	RefreshFailureTotal uint64
	TrackedFiles        int64
	ReauthFiles         int64
	InvalidJSONFiles    int64
	LastScanAt          *time.Time
}

type Registry struct {
	scansTotal          atomic.Uint64
	refreshAttempts     atomic.Uint64
	refreshSuccessTotal atomic.Uint64
	refreshFailureTotal atomic.Uint64
	trackedFiles        atomic.Int64
	reauthFiles         atomic.Int64
	invalidJSONFiles    atomic.Int64
	lastScanUnix        atomic.Int64
}

func New() *Registry {
	return &Registry{}
}

func (r *Registry) IncScans() {
	r.scansTotal.Add(1)
	r.lastScanUnix.Store(time.Now().UTC().Unix())
}

func (r *Registry) IncRefreshAttempts() {
	r.refreshAttempts.Add(1)
}

func (r *Registry) IncRefreshSuccess() {
	r.refreshSuccessTotal.Add(1)
}

func (r *Registry) IncRefreshFailure() {
	r.refreshFailureTotal.Add(1)
}

func (r *Registry) SetTrackedFiles(total, reauth, invalid int) {
	r.trackedFiles.Store(int64(total))
	r.reauthFiles.Store(int64(reauth))
	r.invalidJSONFiles.Store(int64(invalid))
}

func (r *Registry) Snapshot() Snapshot {
	var lastScanAt *time.Time
	if unix := r.lastScanUnix.Load(); unix > 0 {
		value := time.Unix(unix, 0).UTC()
		lastScanAt = &value
	}
	return Snapshot{
		ScansTotal:          r.scansTotal.Load(),
		RefreshAttempts:     r.refreshAttempts.Load(),
		RefreshSuccessTotal: r.refreshSuccessTotal.Load(),
		RefreshFailureTotal: r.refreshFailureTotal.Load(),
		TrackedFiles:        r.trackedFiles.Load(),
		ReauthFiles:         r.reauthFiles.Load(),
		InvalidJSONFiles:    r.invalidJSONFiles.Load(),
		LastScanAt:          lastScanAt,
	}
}

func (r *Registry) RenderPrometheus() string {
	builder := &strings.Builder{}
	fmt.Fprintf(builder, "# HELP codex_auth_scans_total Total auth directory scans.\n")
	fmt.Fprintf(builder, "# TYPE codex_auth_scans_total counter\n")
	fmt.Fprintf(builder, "codex_auth_scans_total %d\n", r.scansTotal.Load())
	fmt.Fprintf(builder, "# HELP codex_auth_refresh_attempts_total Total token refresh attempts.\n")
	fmt.Fprintf(builder, "# TYPE codex_auth_refresh_attempts_total counter\n")
	fmt.Fprintf(builder, "codex_auth_refresh_attempts_total %d\n", r.refreshAttempts.Load())
	fmt.Fprintf(builder, "# HELP codex_auth_refresh_success_total Successful token refresh operations.\n")
	fmt.Fprintf(builder, "# TYPE codex_auth_refresh_success_total counter\n")
	fmt.Fprintf(builder, "codex_auth_refresh_success_total %d\n", r.refreshSuccessTotal.Load())
	fmt.Fprintf(builder, "# HELP codex_auth_refresh_failure_total Failed token refresh operations.\n")
	fmt.Fprintf(builder, "# TYPE codex_auth_refresh_failure_total counter\n")
	fmt.Fprintf(builder, "codex_auth_refresh_failure_total %d\n", r.refreshFailureTotal.Load())
	fmt.Fprintf(builder, "# HELP codex_auth_files_tracked Number of tracked auth files.\n")
	fmt.Fprintf(builder, "# TYPE codex_auth_files_tracked gauge\n")
	fmt.Fprintf(builder, "codex_auth_files_tracked %d\n", r.trackedFiles.Load())
	fmt.Fprintf(builder, "# HELP codex_auth_files_reauth_required Number of auth files requiring re-login.\n")
	fmt.Fprintf(builder, "# TYPE codex_auth_files_reauth_required gauge\n")
	fmt.Fprintf(builder, "codex_auth_files_reauth_required %d\n", r.reauthFiles.Load())
	fmt.Fprintf(builder, "# HELP codex_auth_files_invalid_json Number of auth files with invalid JSON.\n")
	fmt.Fprintf(builder, "# TYPE codex_auth_files_invalid_json gauge\n")
	fmt.Fprintf(builder, "codex_auth_files_invalid_json %d\n", r.invalidJSONFiles.Load())
	fmt.Fprintf(builder, "# HELP codex_auth_last_scan_timestamp_seconds Unix timestamp of the latest scan.\n")
	fmt.Fprintf(builder, "# TYPE codex_auth_last_scan_timestamp_seconds gauge\n")
	fmt.Fprintf(builder, "codex_auth_last_scan_timestamp_seconds %d\n", r.lastScanUnix.Load())
	return builder.String()
}
