import type { DashboardResponse } from '../types';
import { formatAbsolute, formatDuration } from '../utils/time';

import styles from './SummaryCards.module.scss';

export function SummaryCards({ data }: { data: DashboardResponse }) {
  const items = [
    { label: 'Отслеживаемые файлы', value: data.summary.tracked_files, helper: `${data.summary.disabled_files} отключено` },
    { label: 'Исправные', value: data.summary.ok_files, helper: 'Цикл refresh работает стабильно' },
    { label: 'Проблемные', value: data.summary.degraded_files, helper: `${data.metrics.refresh_failure_total} ошибок всего` },
    { label: 'Требуют входа', value: data.summary.reauth_required_files, helper: 'Нужен новый --codex-login' },
    { label: 'Некорректный JSON', value: data.summary.invalid_json_files, helper: 'Парсер не смог прочитать файл' },
    { label: 'Аптайм', value: formatDuration(data.service.uptime_seconds), helper: `Запущен ${formatAbsolute(data.service.started_at)}` },
    { label: 'Последний скан', value: formatAbsolute(data.metrics.last_scan_at), helper: `${data.metrics.scans_total} сканирований всего` },
    { label: 'Успешные refresh', value: data.metrics.refresh_success_total, helper: `${data.metrics.refresh_attempts_total} попыток всего` },
  ];

  return (
    <div className={styles.grid}>
      {items.map((item) => (
        <section key={item.label} className={styles.card}>
          <div className={styles.label}>{item.label}</div>
          <div className={styles.value}>{item.value}</div>
          <div className={styles.helper}>{item.helper}</div>
        </section>
      ))}
    </div>
  );
}
