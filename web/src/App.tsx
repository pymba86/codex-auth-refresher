import { Activity, Files, LayoutDashboard, Link2, RefreshCw, ShieldCheck } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';

import { FileTable } from './components/FileTable';
import { type FileFilter, FiltersBar } from './components/FiltersBar';
import { StatusBanner } from './components/StatusBanner';
import { StatusPill } from './components/StatusPill';
import { SummaryCards } from './components/SummaryCards';
import { useDashboardPoll } from './hooks/useDashboardPoll';
import { formatAbsolute, formatDuration } from './utils/time';
import styles from './App.module.scss';

const FILTER_KEY = 'codex-dashboard-filter';
const SEARCH_KEY = 'codex-dashboard-search';
const SHOW_DISABLED_KEY = 'codex-dashboard-show-disabled';

const PRIORITY: Record<string, number> = {
  reauth_required: 0,
  invalid_json: 1,
  degraded: 2,
  ok: 3,
};

function readStoredBoolean(key: string, fallback: boolean) {
  const value = window.localStorage.getItem(key);
  if (value === null) {
    return fallback;
  }
  return value === 'true';
}

export default function App() {
  const { data, error, isStale, loading, lastUpdatedAt, pollIntervalMs, refresh } = useDashboardPoll();
  const [filter, setFilter] = useState<FileFilter>(() => (window.localStorage.getItem(FILTER_KEY) as FileFilter) || 'all');
  const [search, setSearch] = useState(() => window.localStorage.getItem(SEARCH_KEY) ?? '');
  const [showDisabled, setShowDisabled] = useState(() => readStoredBoolean(SHOW_DISABLED_KEY, true));

  useEffect(() => {
    window.localStorage.setItem(FILTER_KEY, filter);
  }, [filter]);

  useEffect(() => {
    window.localStorage.setItem(SEARCH_KEY, search);
  }, [search]);

  useEffect(() => {
    window.localStorage.setItem(SHOW_DISABLED_KEY, String(showDisabled));
  }, [showDisabled]);

  const filteredFiles = useMemo(() => {
    if (!data) {
      return [];
    }

    const normalized = search.trim().toLowerCase();
    return [...data.files]
      .filter((file) => (showDisabled ? true : !file.disabled))
      .filter((file) => (filter === 'all' ? true : file.state === filter))
      .filter((file) => {
        if (!normalized) {
          return true;
        }
        return file.file.toLowerCase().includes(normalized) || (file.account_id ?? '').toLowerCase().includes(normalized);
      })
      .sort((left, right) => {
        const leftPriority = PRIORITY[left.state] ?? 99;
        const rightPriority = PRIORITY[right.state] ?? 99;
        if (leftPriority !== rightPriority) {
          return leftPriority - rightPriority;
        }
        return left.file.localeCompare(right.file);
      });
  }, [data, filter, search, showDisabled]);

  const endpointItems = [
    ['Dashboard', 'GET /'],
    ['UI JSON', 'GET /v1/dashboard'],
    ['Health', 'GET /healthz'],
    ['Readiness', 'GET /readyz'],
    ['Metrics', 'GET /metrics'],
    ['Raw status', 'GET /v1/status'],
  ];

  const summaryContent = data ? <SummaryCards data={data} /> : null;
  const errorMessage = error?.replace('dashboard_request_failed:', 'HTTP ');

  return (
    <div className={styles.shell}>
      <aside className={styles.sidebar}>
        <div className={styles.brand}>
          <div className={styles.brandIcon}>
            <ShieldCheck size={22} />
          </div>
          <div>
            <div className={styles.brandTitle}>codex-auth-refresher</div>
            <div className={styles.brandSubtitle}>Status Dashboard</div>
          </div>
        </div>

        <nav className={styles.nav}>
          <a href="#overview"><LayoutDashboard size={18} /> Overview</a>
          <a href="#files"><Files size={18} /> Files</a>
          <a href="#metrics"><Activity size={18} /> Metrics</a>
          <a href="#endpoints"><Link2 size={18} /> Endpoints</a>
        </nav>

        {data && (
          <div className={styles.stack}>
            <div className={styles.sidebarCard}>
              <div className={styles.sidebarLabel}>Runtime policy</div>
              <div className={styles.sidebarValue}>{data.config.refresh_before} / {data.config.refresh_max_age}</div>
              <div className={styles.sidebarHint}>
                Scan interval {data.config.scan_interval}, max parallel {data.config.max_parallel}.
              </div>
            </div>
            <div className={styles.sidebarCard}>
              <div className={styles.sidebarLabel}>Service state</div>
              <div className={styles.sidebarValue}><StatusPill state={data.service.ready ? 'ok' : 'degraded'} /></div>
              <div className={styles.sidebarHint}>Uptime {formatDuration(data.service.uptime_seconds)}.</div>
            </div>
          </div>
        )}
      </aside>

      <main className={styles.content}>
        <div className={styles.topbar}>
          <div>
            <div className={styles.title}>Codex token fleet at a glance</div>
            <div className={styles.subtitle}>
              A read-only operations dashboard for refresh health, file state, and upcoming token maintenance windows.
            </div>
          </div>
          <div className={styles.topMeta}>
            <div className={styles.metricPill}>Polling every {Math.round(pollIntervalMs / 1000)}s</div>
            {lastUpdatedAt && <div className={styles.metricPill}>Updated {lastUpdatedAt.toLocaleTimeString()}</div>}
            <button type="button" className={styles.refreshButton} onClick={() => void refresh()} disabled={loading}>
              <RefreshCw size={18} /> Refresh
            </button>
          </div>
        </div>

        {!data && loading && (
          <StatusBanner
            title="Loading dashboard"
            message="Fetching current refresh state, file inventory, and operational metrics."
          />
        )}

        {data && !data.service.ready && (
          <StatusBanner title="Service is starting" message="The scheduler is still warming up. Data may be incomplete until readiness turns green." />
        )}

        {errorMessage && (
          <StatusBanner
            title={isStale ? 'Showing last known data' : 'Dashboard fetch failed'}
            message={isStale ? `Latest API request failed (${errorMessage}), but the last successful snapshot stays on screen.` : `The dashboard API request failed: ${errorMessage}.`}
            variant="warning"
          />
        )}

        {data && (
          <>
            <section id="overview" className={styles.section}>
              <div className={styles.sectionTitle}>Overview</div>
              {summaryContent}
            </section>

            <section id="files" className={styles.section}>
              <div className={styles.sectionTitle}>Files</div>
              <FiltersBar
                filter={filter}
                onFilterChange={setFilter}
                search={search}
                onSearchChange={setSearch}
                showDisabled={showDisabled}
                onShowDisabledChange={setShowDisabled}
              />
              <div style={{ marginTop: '1rem' }}>
                <FileTable files={filteredFiles} />
              </div>
            </section>

            <section id="metrics" className={styles.section}>
              <div className={styles.sectionTitle}>Metrics</div>
              <div className={styles.metricsGrid}>
                <div className={styles.metricsCard}>
                  <div className={styles.metricsCardLabel}>Refresh attempts</div>
                  <div className={styles.metricsCardValue}>{data.metrics.refresh_attempts_total}</div>
                  <div className={styles.metricsCardHint}>Success {data.metrics.refresh_success_total} · Failure {data.metrics.refresh_failure_total}</div>
                </div>
                <div className={styles.metricsCard}>
                  <div className={styles.metricsCardLabel}>Scans total</div>
                  <div className={styles.metricsCardValue}>{data.metrics.scans_total}</div>
                  <div className={styles.metricsCardHint}>Last scan {formatAbsolute(data.metrics.last_scan_at)}</div>
                </div>
                <div className={styles.metricsCard}>
                  <div className={styles.metricsCardLabel}>Readiness</div>
                  <div className={styles.metricsCardValue}>{data.service.ready ? 'Ready' : 'Starting'}</div>
                  <div className={styles.metricsCardHint}>Started {formatAbsolute(data.service.started_at)}</div>
                </div>
                <div className={styles.metricsCard}>
                  <div className={styles.metricsCardLabel}>Status API</div>
                  <div className={styles.metricsCardValue}>{data.config.status_api_enabled ? 'Enabled' : 'Disabled'}</div>
                  <div className={styles.metricsCardHint}>Raw JSON endpoint availability</div>
                </div>
              </div>
            </section>

            <section id="endpoints" className={styles.section}>
              <div className={styles.sectionTitle}>Endpoints</div>
              <div className={styles.endpointsCard}>
                <ul>
                  {endpointItems.map(([label, value]) => (
                    <li key={value}>
                      <strong>{label}:</strong> <code>{value}</code>
                    </li>
                  ))}
                </ul>
              </div>
            </section>
          </>
        )}
      </main>
    </div>
  );
}
