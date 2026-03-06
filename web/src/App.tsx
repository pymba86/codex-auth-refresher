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

function localizeError(error: string | null): string | null {
  if (!error) {
    return null;
  }
  if (error.startsWith('dashboard_request_failed:')) {
    return `HTTP ${error.slice('dashboard_request_failed:'.length)}`;
  }
  if (error === 'unknown_dashboard_error') {
    return 'неизвестная ошибка дашборда';
  }
  return error;
}

function localizeConfigValue(value: string): string {
  if (value === 'disabled') {
    return 'выключено';
  }
  return value;
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
    ['Дашборд', 'GET /'],
    ['JSON для UI', 'GET /v1/dashboard'],
    ['Проверка жизни', 'GET /healthz'],
    ['Готовность', 'GET /readyz'],
    ['Метрики', 'GET /metrics'],
    ['Сырой статус', 'GET /v1/status'],
  ];

  const summaryContent = data ? <SummaryCards data={data} /> : null;
  const errorMessage = localizeError(error);

  return (
    <div className={styles.shell}>
      <aside className={styles.sidebar}>
        <div className={styles.brand}>
          <div className={styles.brandIcon}>
            <ShieldCheck size={22} />
          </div>
          <div>
            <div className={styles.brandTitle}>codex-auth-refresher</div>
            <div className={styles.brandSubtitle}>Панель состояния</div>
          </div>
        </div>

        <nav className={styles.nav}>
          <a href="#overview"><LayoutDashboard size={18} /> Обзор</a>
          <a href="#files"><Files size={18} /> Файлы</a>
          <a href="#metrics"><Activity size={18} /> Метрики</a>
          <a href="#endpoints"><Link2 size={18} /> Эндпоинты</a>
        </nav>

        {data && (
          <div className={styles.stack}>
            <div className={styles.sidebarCard}>
              <div className={styles.sidebarLabel}>Политика обновления</div>
              <div className={styles.sidebarValue}>
                {data.config.refresh_before} / {localizeConfigValue(data.config.refresh_max_age)}
              </div>
              <div className={styles.sidebarHint}>
                Интервал сканирования {data.config.scan_interval}, параллелизм {data.config.max_parallel}.
              </div>
            </div>
            <div className={styles.sidebarCard}>
              <div className={styles.sidebarLabel}>Состояние сервиса</div>
              <div className={styles.sidebarValue}><StatusPill state={data.service.ready ? 'ok' : 'degraded'} /></div>
              <div className={styles.sidebarHint}>Аптайм {formatDuration(data.service.uptime_seconds)}.</div>
            </div>
          </div>
        )}
      </aside>

      <main className={styles.content}>
        <div className={styles.topbar}>
          <div>
            <div className={styles.title}>Состояние Codex-токенов в одном окне</div>
            <div className={styles.subtitle}>
              Панель только для чтения: видно здоровье refresh-цикла, состояние auth-файлов и ближайшие обновления.
            </div>
          </div>
          <div className={styles.topMeta}>
            <div className={styles.metricPill}>Автообновление каждые {Math.round(pollIntervalMs / 1000)} с</div>
            {lastUpdatedAt && <div className={styles.metricPill}>Обновлено {lastUpdatedAt.toLocaleTimeString('ru-RU')}</div>}
            <button type="button" className={styles.refreshButton} onClick={() => void refresh()} disabled={loading}>
              <RefreshCw size={18} /> Обновить
            </button>
          </div>
        </div>

        {!data && loading && (
          <StatusBanner
            title="Загрузка дашборда"
            message="Получаю текущее состояние refresh-цикла, список файлов и операционные метрики."
          />
        )}

        {data && !data.service.ready && (
          <StatusBanner title="Сервис запускается" message="Планировщик ещё прогревается. Пока readiness не станет зелёным, данные могут быть неполными." />
        )}

        {errorMessage && (
          <StatusBanner
            title={isStale ? 'Показываю последние успешные данные' : 'Не удалось обновить дашборд'}
            message={
              isStale
                ? `Последний запрос к API завершился ошибкой (${errorMessage}), но на экране остаётся предыдущий успешный снимок.`
                : `Запрос к dashboard API завершился ошибкой: ${errorMessage}.`
            }
            variant="warning"
          />
        )}

        {data && (
          <>
            <section id="overview" className={styles.section}>
              <div className={styles.sectionTitle}>Обзор</div>
              {summaryContent}
            </section>

            <section id="files" className={styles.section}>
              <div className={styles.sectionTitle}>Файлы</div>
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
              <div className={styles.sectionTitle}>Метрики</div>
              <div className={styles.metricsGrid}>
                <div className={styles.metricsCard}>
                  <div className={styles.metricsCardLabel}>Попытки refresh</div>
                  <div className={styles.metricsCardValue}>{data.metrics.refresh_attempts_total}</div>
                  <div className={styles.metricsCardHint}>Успешно {data.metrics.refresh_success_total} · Ошибок {data.metrics.refresh_failure_total}</div>
                </div>
                <div className={styles.metricsCard}>
                  <div className={styles.metricsCardLabel}>Сканирований</div>
                  <div className={styles.metricsCardValue}>{data.metrics.scans_total}</div>
                  <div className={styles.metricsCardHint}>Последний скан {formatAbsolute(data.metrics.last_scan_at)}</div>
                </div>
                <div className={styles.metricsCard}>
                  <div className={styles.metricsCardLabel}>Готовность</div>
                  <div className={styles.metricsCardValue}>{data.service.ready ? 'Готов' : 'Запуск'}</div>
                  <div className={styles.metricsCardHint}>Старт {formatAbsolute(data.service.started_at)}</div>
                </div>
                <div className={styles.metricsCard}>
                  <div className={styles.metricsCardLabel}>Status API</div>
                  <div className={styles.metricsCardValue}>{data.config.status_api_enabled ? 'Включён' : 'Выключен'}</div>
                  <div className={styles.metricsCardHint}>Доступность сырого JSON-статуса</div>
                </div>
              </div>
            </section>

            <section id="endpoints" className={styles.section}>
              <div className={styles.sectionTitle}>Эндпоинты</div>
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
