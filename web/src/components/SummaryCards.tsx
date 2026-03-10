import type { DashboardResponse } from '../types';
import { useI18n } from '../i18n';
import { formatAbsolute, formatDuration, formatRelative } from '../utils/time';

import styles from './SummaryCards.module.scss';

export function SummaryCards({ data }: { data: DashboardResponse }) {
  const { locale, t } = useI18n();

  const items = [
    {
      key: 'tracked',
      label: t('summary.trackedFiles'),
      value: data.summary.tracked_files,
      helper: t('summary.trackedFilesHelper', { count: data.summary.disabled_files }),
    },
    {
      key: 'ok',
      label: t('summary.okFiles'),
      value: data.summary.ok_files,
      helper: t('summary.okFilesHelper'),
    },
    {
      key: 'degraded',
      label: t('summary.degradedFiles'),
      value: data.summary.degraded_files,
      helper: t('summary.degradedFilesHelper', { count: data.metrics.refresh_failure_total }),
    },
    {
      key: 'reauth',
      label: t('summary.reauthFiles'),
      value: data.summary.reauth_required_files,
      helper: t('summary.reauthFilesHelper'),
    },
    {
      key: 'invalid',
      label: t('summary.invalidJsonFiles'),
      value: data.summary.invalid_json_files,
      helper: t('summary.invalidJsonFilesHelper'),
    },
    {
      key: 'uptime',
      label: t('summary.uptime'),
      value: formatDuration(data.service.uptime_seconds, locale),
      helper: t('summary.uptimeHelper', { time: formatAbsolute(data.service.started_at, locale) }),
    },
    {
      key: 'scan',
      label: t('summary.lastScan'),
      value: formatRelative(data.metrics.last_scan_at, locale),
      helper: t('summary.lastScanHelper', { count: data.metrics.scans_total }),
    },
    {
      key: 'success',
      label: t('summary.refreshSuccess'),
      value: data.metrics.refresh_success_total,
      helper: t('summary.refreshSuccessHelper', { count: data.metrics.refresh_attempts_total }),
    },
  ];

  return (
    <div className={styles.grid}>
      {items.map((item) => (
        <section key={item.key} className={styles.card}>
          <div className={styles.label}>{item.label}</div>
          <div className={styles.value}>{item.value}</div>
          <div className={styles.helper}>{item.helper}</div>
        </section>
      ))}
    </div>
  );
}
