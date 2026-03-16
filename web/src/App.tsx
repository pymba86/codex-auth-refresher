import { Activity, Files, LayoutDashboard, Link2, RefreshCw, ShieldCheck } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';

import { FileTable } from './components/FileTable';
import { type FileFilter, FiltersBar } from './components/FiltersBar';
import { StatusBanner } from './components/StatusBanner';
import { SummaryCards } from './components/SummaryCards';
import { type Locale, type TranslationKey, useI18n } from './i18n';
import { useDashboardPoll } from './hooks/useDashboardPoll';
import { formatAbsolute, formatClockTime, formatDuration } from './utils/time';
import styles from './App.module.scss';

const FILTER_KEY = 'codex-dashboard-filter';
const SEARCH_KEY = 'codex-dashboard-search';
const SHOW_DISABLED_KEY = 'codex-dashboard-show-disabled';

const SUPPORTED_LOCALES: Locale[] = ['ru', 'en'];
const FILE_FILTERS: FileFilter[] = ['all', 'ok', 'degraded', 'reauth_required', 'invalid_json'];

const PRIORITY: Record<string, number> = {
  reauth_required: 0,
  invalid_json: 1,
  degraded: 2,
  ok: 3,
};

const ENDPOINT_ITEMS: Array<{ labelKey: TranslationKey; value: string }> = [
  { labelKey: 'endpoint.dashboard', value: 'GET /' },
  { labelKey: 'endpoint.uiJson', value: 'GET /v1/dashboard' },
  { labelKey: 'endpoint.healthz', value: 'GET /healthz' },
  { labelKey: 'endpoint.readyz', value: 'GET /readyz' },
  { labelKey: 'endpoint.metrics', value: 'GET /metrics' },
  { labelKey: 'endpoint.rawStatus', value: 'GET /v1/status' },
];

type TranslateFn = (key: TranslationKey, params?: Record<string, number | string>) => string;

function readStoredBoolean(key: string, fallback: boolean) {
  const value = window.localStorage.getItem(key);
  if (value === null) {
    return fallback;
  }
  return value === 'true';
}

function readStoredFilter(): FileFilter {
  const value = window.localStorage.getItem(FILTER_KEY);
  return FILE_FILTERS.includes(value as FileFilter) ? (value as FileFilter) : 'all';
}

function localizeError(error: string | null, t: TranslateFn): string | null {
  if (!error) {
    return null;
  }
  if (error.startsWith('dashboard_request_failed:')) {
    return t('error.httpStatus', { status: error.slice('dashboard_request_failed:'.length) });
  }
  if (error === 'unknown_dashboard_error') {
    return t('error.unknownDashboard');
  }
  return error;
}

function localizeConfigValue(value: string, t: TranslateFn): string {
  if (value === 'disabled') {
    return t('common.disabledValue');
  }
  return value;
}

export default function App() {
  const { locale, setLocale, t } = useI18n();
  const { data, error, isStale, loading, lastUpdatedAt, pollIntervalMs, refresh } = useDashboardPoll();
  const [filter, setFilter] = useState<FileFilter>(() => readStoredFilter());
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
        return (
          file.file.toLowerCase().includes(normalized) ||
          (file.type ?? '').toLowerCase().includes(normalized) ||
          (file.account_id ?? '').toLowerCase().includes(normalized)
        );
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

  const errorMessage = localizeError(error, t);

  return (
    <div className={styles.shell}>
      <aside className={styles.sidebar}>
        <div className={styles.brand}>
          <div className={styles.brandIcon}>
            <ShieldCheck size={22} />
          </div>
          <div>
            <div className={styles.brandTitle}>codex-auth-refresher</div>
            <div className={styles.brandSubtitle}>{t('app.brandSubtitle')}</div>
          </div>
        </div>

        <nav className={styles.nav}>
          <a href="#overview"><LayoutDashboard size={18} /> {t('nav.overview')}</a>
          <a href="#files"><Files size={18} /> {t('nav.files')}</a>
          <a href="#metrics"><Activity size={18} /> {t('nav.metrics')}</a>
          <a href="#endpoints"><Link2 size={18} /> {t('nav.endpoints')}</a>
        </nav>

        {data && (
          <div className={styles.stack}>
            <div className={styles.sidebarCard}>
              <div className={styles.sidebarLabel}>{t('sidebar.refreshPolicy')}</div>
              <div className={styles.sidebarValue}>
                {data.config.refresh_before} / {localizeConfigValue(data.config.refresh_max_age, t)}
              </div>
              <div className={styles.sidebarHint}>
                {t('sidebar.scanPolicy', {
                  interval: data.config.scan_interval,
                  parallel: data.config.max_parallel,
                })}
              </div>
            </div>
            <div className={styles.sidebarCard}>
              <div className={styles.sidebarLabel}>{t('sidebar.serviceState')}</div>
              <div className={styles.sidebarValue}>{data.service.ready ? t('metrics.ready') : t('metrics.starting')}</div>
              <div className={styles.sidebarHint}>{t('sidebar.serviceUptime', { value: formatDuration(data.service.uptime_seconds, locale) })}</div>
            </div>
          </div>
        )}
      </aside>

      <main className={styles.content}>
        <div className={styles.topbar}>
          <div className={styles.hero}>
            <div className={styles.title}>{t('app.title')}</div>
            <div className={styles.subtitle}>{t('app.subtitle')}</div>
          </div>
          <div className={styles.topMeta}>
            <div className={styles.metricPill}>{t('top.autoRefresh', { seconds: Math.round(pollIntervalMs / 1000) })}</div>
            {lastUpdatedAt && <div className={styles.metricPill}>{t('top.updatedAt', { time: formatClockTime(lastUpdatedAt, locale) })}</div>}
            <div className={styles.localeSwitch} role="group" aria-label={t('top.languageSwitcher')}>
              {SUPPORTED_LOCALES.map((value) => (
                <button
                  key={value}
                  type="button"
                  className={`${styles.localeButton} ${locale === value ? styles.localeButtonActive : ''}`}
                  onClick={() => setLocale(value)}
                  aria-pressed={locale === value}
                >
                  {value.toUpperCase()}
                </button>
              ))}
            </div>
            <button type="button" className={styles.refreshButton} onClick={() => void refresh()} disabled={loading}>
              <RefreshCw size={18} /> {t('top.refresh')}
            </button>
          </div>
        </div>

        {!data && loading && (
          <StatusBanner
            title={t('banner.loadingTitle')}
            message={t('banner.loadingMessage')}
          />
        )}

        {data && !data.service.ready && (
          <StatusBanner title={t('banner.startingTitle')} message={t('banner.startingMessage')} />
        )}

        {errorMessage && (
          <StatusBanner
            title={isStale ? t('banner.staleTitle') : t('banner.fetchFailedTitle')}
            message={isStale ? t('banner.staleMessage', { error: errorMessage }) : t('banner.fetchFailedMessage', { error: errorMessage })}
            variant="warning"
          />
        )}

        {data && (
          <>
            <section id="overview" className={styles.section}>
              <div className={styles.sectionTitle}>{t('section.overview')}</div>
              <SummaryCards data={data} />
            </section>

            <section id="files" className={styles.section}>
              <div className={styles.sectionTitle}>{t('section.files')}</div>
              <FiltersBar
                filter={filter}
                onFilterChange={setFilter}
                search={search}
                onSearchChange={setSearch}
                showDisabled={showDisabled}
                onShowDisabledChange={setShowDisabled}
              />
              <div className={styles.tableSection}>
                <FileTable files={filteredFiles} />
              </div>
            </section>

            <section id="metrics" className={styles.section}>
              <div className={styles.sectionTitle}>{t('section.metrics')}</div>
              <div className={styles.metricsGrid}>
                <div className={styles.metricsCard}>
                  <div className={styles.metricsCardLabel}>{t('metrics.refreshAttempts')}</div>
                  <div className={styles.metricsCardValue}>{data.metrics.refresh_attempts_total}</div>
                  <div className={styles.metricsCardHint}>
                    {t('metrics.refreshAttemptsHint', {
                      success: data.metrics.refresh_success_total,
                      failure: data.metrics.refresh_failure_total,
                    })}
                  </div>
                </div>
                <div className={styles.metricsCard}>
                  <div className={styles.metricsCardLabel}>{t('metrics.scans')}</div>
                  <div className={styles.metricsCardValue}>{data.metrics.scans_total}</div>
                  <div className={styles.metricsCardHint}>{t('metrics.scansHint', { time: formatAbsolute(data.metrics.last_scan_at, locale) })}</div>
                </div>
                <div className={styles.metricsCard}>
                  <div className={styles.metricsCardLabel}>{t('metrics.readiness')}</div>
                  <div className={styles.metricsCardValue}>{data.service.ready ? t('metrics.ready') : t('metrics.starting')}</div>
                  <div className={styles.metricsCardHint}>{t('metrics.startedAt', { time: formatAbsolute(data.service.started_at, locale) })}</div>
                </div>
                <div className={styles.metricsCard}>
                  <div className={styles.metricsCardLabel}>{t('metrics.statusApi')}</div>
                  <div className={styles.metricsCardValue}>{data.config.status_api_enabled ? t('common.enabled') : t('common.disabled')}</div>
                  <div className={styles.metricsCardHint}>{t('metrics.statusApiHint')}</div>
                </div>
              </div>
            </section>

            <section id="endpoints" className={styles.section}>
              <div className={styles.sectionTitle}>{t('section.endpoints')}</div>
              <div className={styles.endpointsCard}>
                <ul>
                  {ENDPOINT_ITEMS.map((item) => (
                    <li key={item.value}>
                      <strong>{t(item.labelKey)}:</strong> <code>{item.value}</code>
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
