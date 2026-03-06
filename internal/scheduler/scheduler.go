package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"codex-auth-refresher/internal/metrics"
	"codex-auth-refresher/internal/oauth"
	"codex-auth-refresher/internal/refresher"
	"codex-auth-refresher/internal/watch"
)

type FileStatus struct {
	File                string              `json:"file"`
	AccountID           string              `json:"account_id,omitempty"`
	Schema              string              `json:"schema,omitempty"`
	State               refresher.FileState `json:"state"`
	ExpiresAt           *time.Time          `json:"expires_at,omitempty"`
	NextRefreshAt       *time.Time          `json:"next_refresh_at,omitempty"`
	LastRefreshAt       *time.Time          `json:"last_refresh_at,omitempty"`
	ConsecutiveFailures int                 `json:"consecutive_failures"`
	LastError           string              `json:"last_error,omitempty"`
	Disabled            bool                `json:"disabled,omitempty"`
}

type Snapshot struct {
	StartedAt time.Time    `json:"started_at"`
	AuthDir   string       `json:"auth_dir"`
	Files     []FileStatus `json:"files"`
}

type fileRecord struct {
	status        FileStatus
	accountKey    string
	busy          bool
	nextAttemptAt time.Time
	modTime       time.Time
}

type refreshJob struct {
	path       string
	accountKey string
}

type directoryWatcher interface {
	Events() <-chan struct{}
	Errors() <-chan error
	Close() error
}

type Manager struct {
	authDir      string
	scanInterval time.Duration
	maxParallel  int
	refresher    *refresher.Service
	metrics      *metrics.Registry
	logger       *slog.Logger
	startedAt    time.Time
	watchFactory func(string) (directoryWatcher, error)

	mu           sync.RWMutex
	ready        bool
	files        map[string]*fileRecord
	accountsBusy map[string]bool
	jobs         chan refreshJob
}

func NewManager(authDir string, scanInterval time.Duration, maxParallel int, refreshService *refresher.Service, metricsRegistry *metrics.Registry, logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		authDir:      authDir,
		scanInterval: scanInterval,
		maxParallel:  maxParallel,
		refresher:    refreshService,
		metrics:      metricsRegistry,
		logger:       logger,
		startedAt:    time.Now().UTC(),
		watchFactory: func(dir string) (directoryWatcher, error) { return watch.New(dir) },
		files:        make(map[string]*fileRecord),
		accountsBusy: make(map[string]bool),
		jobs:         make(chan refreshJob, maxParallel*4),
	}
}

func (m *Manager) Run(ctx context.Context) error {
	for i := 0; i < m.maxParallel; i++ {
		go m.worker(ctx)
	}
	if err := m.runScan(ctx); err != nil {
		return err
	}

	var watcher directoryWatcher
	var watchEvents <-chan struct{}
	var watchErrors <-chan error
	if m.watchFactory != nil {
		createdWatcher, err := m.watchFactory(m.authDir)
		if err != nil {
			m.logger.Warn("watcher disabled", "error", err)
		} else {
			watcher = createdWatcher
			watchEvents = watcher.Events()
			watchErrors = watcher.Errors()
			defer watcher.Close()
		}
	}

	ticker := time.NewTicker(m.scanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := m.runScan(ctx); err != nil {
				m.logger.Error("scan failed", "error", err)
			}
		case _, ok := <-watchEvents:
			if !ok {
				watchEvents = nil
				continue
			}
			if err := m.runScan(ctx); err != nil {
				m.logger.Error("watch-triggered scan failed", "error", err)
			}
		case err, ok := <-watchErrors:
			if !ok {
				watchErrors = nil
				continue
			}
			m.logger.Warn("watcher error", "error", err)
		}
	}
}

func (m *Manager) Ready() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.ready
}

func (m *Manager) Snapshot() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	files := make([]FileStatus, 0, len(m.files))
	for _, record := range m.files {
		files = append(files, cloneStatus(record.status))
	}
	sort.Slice(files, func(i, j int) bool { return files[i].File < files[j].File })
	return Snapshot{StartedAt: m.startedAt, AuthDir: m.authDir, Files: files}
}

func (m *Manager) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-m.jobs:
			m.handleJob(ctx, job)
		}
	}
}

func (m *Manager) handleJob(ctx context.Context, job refreshJob) {
	m.metrics.IncRefreshAttempts()
	result, err := m.refresher.RefreshFile(ctx, job.path)

	m.mu.Lock()
	defer m.mu.Unlock()
	record := m.files[job.path]
	if record == nil {
		delete(m.accountsBusy, job.accountKey)
		return
	}
	record.busy = false
	delete(m.accountsBusy, job.accountKey)
	if err != nil {
		m.metrics.IncRefreshFailure()
		record.status.LastError = sanitizeError(err)
		record.status.ConsecutiveFailures++
		record.status.Disabled = result.Inspection.Disabled
		record.status.AccountID = result.Inspection.AccountID
		record.status.Schema = result.Inspection.Schema
		record.status.ExpiresAt = cloneTime(result.Inspection.ExpiresAt)
		record.status.NextRefreshAt = cloneTime(result.Inspection.NextRefreshAt)
		record.status.LastRefreshAt = cloneTime(result.Inspection.LastRefreshAt)
		record.accountKey = result.Inspection.AccountKey
		record.status.State = classifyState(err)
		record.nextAttemptAt = nextAttemptTime(record.status.ConsecutiveFailures, record.status.State)
		m.updateMetricsLocked()
		m.logger.Warn("refresh failed", "file", filepath.Base(job.path), "state", record.status.State, "error", record.status.LastError)
		return
	}

	record.status = FileStatus{
		File:                result.Inspection.File,
		AccountID:           result.Inspection.AccountID,
		Schema:              result.Inspection.Schema,
		State:               refresher.StateOK,
		ExpiresAt:           cloneTime(result.Inspection.ExpiresAt),
		NextRefreshAt:       cloneTime(result.Inspection.NextRefreshAt),
		LastRefreshAt:       cloneTime(result.Inspection.LastRefreshAt),
		ConsecutiveFailures: 0,
		Disabled:            result.Inspection.Disabled,
	}
	record.accountKey = result.Inspection.AccountKey
	record.nextAttemptAt = time.Time{}
	m.metrics.IncRefreshSuccess()
	m.updateMetricsLocked()
	m.logger.Info("refresh succeeded", "file", filepath.Base(job.path), "account_id", result.Inspection.AccountID)
}

func (m *Manager) runScan(ctx context.Context) error {
	if err := m.scanOnce(ctx); err != nil {
		return err
	}
	return nil
}

func (m *Manager) scanOnce(ctx context.Context) error {
	entries, err := os.ReadDir(m.authDir)
	if err != nil {
		return fmt.Errorf("read auth dir: %w", err)
	}
	m.metrics.IncScans()
	seen := make(map[string]bool, len(entries))
	jobs := make([]refreshJob, 0)
	now := time.Now().UTC()

	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		path := filepath.Join(m.authDir, entry.Name())
		info, infoErr := entry.Info()
		if infoErr != nil {
			m.logger.Warn("stat auth file failed", "file", entry.Name(), "error", infoErr)
			continue
		}
		seen[path] = true
		inspection, inspectErr := m.refresher.InspectFile(path)

		m.mu.Lock()
		record := m.ensureRecordLocked(path, entry.Name())
		changed := !record.modTime.Equal(info.ModTime())
		record.modTime = info.ModTime()
		if changed && (record.status.State == refresher.StateReauthRequired || record.status.State == refresher.StateInvalidJSON) {
			record.status.ConsecutiveFailures = 0
			record.status.LastError = ""
			record.nextAttemptAt = time.Time{}
		}
		if inspectErr != nil {
			record.status.File = entry.Name()
			record.status.State = refresher.StateInvalidJSON
			record.status.LastError = sanitizeError(inspectErr)
			record.status.ConsecutiveFailures = max(record.status.ConsecutiveFailures, 1)
			record.accountKey = path
			m.mu.Unlock()
			continue
		}

		consecutiveFailures := record.status.ConsecutiveFailures
		lastError := record.status.LastError
		record.status = FileStatus{
			File:                inspection.File,
			AccountID:           inspection.AccountID,
			Schema:              inspection.Schema,
			State:               refresher.StateOK,
			ExpiresAt:           cloneTime(inspection.ExpiresAt),
			NextRefreshAt:       cloneTime(inspection.NextRefreshAt),
			LastRefreshAt:       cloneTime(inspection.LastRefreshAt),
			ConsecutiveFailures: consecutiveFailures,
			LastError:           lastError,
			Disabled:            inspection.Disabled,
		}
		if consecutiveFailures > 0 {
			record.status.State = refresher.StateDegraded
		}
		record.accountKey = inspection.AccountKey
		shouldQueue := inspection.RefreshDue && !inspection.Disabled && inspection.RefreshTokenPresent && !record.busy && !m.accountsBusy[record.accountKey] && (record.nextAttemptAt.IsZero() || !record.nextAttemptAt.After(now))
		if shouldQueue {
			record.busy = true
			m.accountsBusy[record.accountKey] = true
			jobs = append(jobs, refreshJob{path: path, accountKey: record.accountKey})
		}
		m.mu.Unlock()
	}

	m.mu.Lock()
	for path, record := range m.files {
		if !seen[path] && !record.busy {
			delete(m.files, path)
		}
	}
	m.ready = true
	m.updateMetricsLocked()
	m.mu.Unlock()

	for _, job := range jobs {
		select {
		case <-ctx.Done():
			return nil
		case m.jobs <- job:
		}
	}
	return nil
}

func (m *Manager) ensureRecordLocked(path, filename string) *fileRecord {
	record, ok := m.files[path]
	if ok {
		return record
	}
	record = &fileRecord{status: FileStatus{File: filename, State: refresher.StateOK}, accountKey: path}
	m.files[path] = record
	return record
}

func (m *Manager) updateMetricsLocked() {
	total := len(m.files)
	reauth := 0
	invalid := 0
	for _, record := range m.files {
		switch record.status.State {
		case refresher.StateReauthRequired:
			reauth++
		case refresher.StateInvalidJSON:
			invalid++
		}
	}
	m.metrics.SetTrackedFiles(total, reauth, invalid)
}

func cloneStatus(status FileStatus) FileStatus {
	return FileStatus{
		File:                status.File,
		AccountID:           status.AccountID,
		Schema:              status.Schema,
		State:               status.State,
		ExpiresAt:           cloneTime(status.ExpiresAt),
		NextRefreshAt:       cloneTime(status.NextRefreshAt),
		LastRefreshAt:       cloneTime(status.LastRefreshAt),
		ConsecutiveFailures: status.ConsecutiveFailures,
		LastError:           status.LastError,
		Disabled:            status.Disabled,
	}
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := value.UTC()
	return &copy
}

func classifyState(err error) refresher.FileState {
	var oauthErr *oauth.Error
	if errors.As(err, &oauthErr) {
		if oauthErr.InvalidGrant() {
			return refresher.StateReauthRequired
		}
		return refresher.StateDegraded
	}
	return refresher.StateDegraded
}

func nextAttemptTime(failures int, state refresher.FileState) time.Time {
	if state == refresher.StateReauthRequired {
		return time.Now().UTC().Add(time.Hour)
	}
	backoffs := []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, 30 * time.Minute, time.Hour}
	idx := failures - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(backoffs) {
		idx = len(backoffs) - 1
	}
	base := backoffs[idx]
	jitter := time.Duration(rand.Int63n(int64(base) / 5))
	return time.Now().UTC().Add(base - base/10 + jitter)
}

func sanitizeError(err error) string {
	msg := strings.TrimSpace(err.Error())
	if len(msg) > 200 {
		return msg[:200]
	}
	return msg
}
