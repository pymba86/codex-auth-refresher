import type { DashboardResponse } from '../types';
import { formatAbsolute, formatDuration } from '../utils/time';

import styles from './SummaryCards.module.scss';

export function SummaryCards({ data }: { data: DashboardResponse }) {
  const items = [
    { label: 'Tracked files', value: data.summary.tracked_files, helper: `${data.summary.disabled_files} disabled` },
    { label: 'Healthy', value: data.summary.ok_files, helper: 'Refresh loop is green' },
    { label: 'Degraded', value: data.summary.degraded_files, helper: `${data.metrics.refresh_failure_total} total failures` },
    { label: 'Reauth needed', value: data.summary.reauth_required_files, helper: 'Requires fresh --codex-login' },
    { label: 'Invalid JSON', value: data.summary.invalid_json_files, helper: 'Files unreadable by parser' },
    { label: 'Uptime', value: formatDuration(data.service.uptime_seconds), helper: `Started ${formatAbsolute(data.service.started_at)}` },
    { label: 'Last scan', value: formatAbsolute(data.metrics.last_scan_at), helper: `${data.metrics.scans_total} scans total` },
    { label: 'Refresh success', value: data.metrics.refresh_success_total, helper: `${data.metrics.refresh_attempts_total} attempts total` },
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
