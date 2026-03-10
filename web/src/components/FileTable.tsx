import type { ReactNode } from 'react';

import type { DashboardFile } from '../types';
import { type TranslationKey, useI18n } from '../i18n';
import { formatAbsolute, formatRelative } from '../utils/time';

import { StatusPill } from './StatusPill';
import styles from './FileTable.module.scss';

type FileColumn = {
  key: string;
  labelKey: TranslationKey;
};

const COLUMNS: FileColumn[] = [
  { key: 'file', labelKey: 'files.column.file' },
  { key: 'account', labelKey: 'files.column.account' },
  { key: 'schema', labelKey: 'files.column.schema' },
  { key: 'state', labelKey: 'files.column.state' },
  { key: 'expiresAt', labelKey: 'files.column.expiresAt' },
  { key: 'nextRefreshAt', labelKey: 'files.column.nextRefreshAt' },
  { key: 'lastRefreshAt', labelKey: 'files.column.lastRefreshAt' },
  { key: 'consecutiveFailures', labelKey: 'files.column.consecutiveFailures' },
  { key: 'disabled', labelKey: 'files.column.disabled' },
  { key: 'error', labelKey: 'files.column.error' },
];

export function FileTable({ files }: { files: DashboardFile[] }) {
  const { locale, t } = useI18n();

  const renderTimestamp = (value?: string) => (
    <>
      <div className={styles.primary}>{formatRelative(value, locale)}</div>
      <div className={styles.subtle}>{formatAbsolute(value, locale)}</div>
    </>
  );

  const renderCell = (file: DashboardFile, columnKey: string): ReactNode => {
    switch (columnKey) {
      case 'file':
        return <div className={`${styles.primary} ${styles.wrapAnywhere}`}>{file.file}</div>;
      case 'account':
        return <div className={`${styles.primary} ${styles.wrapAnywhere}`}>{file.account_id ?? t('common.notAvailable')}</div>;
      case 'schema':
        return <div className={`${styles.primary} ${styles.wrapAnywhere}`}>{file.schema ?? t('common.notAvailable')}</div>;
      case 'state':
        return <StatusPill state={file.state} />;
      case 'expiresAt':
        return renderTimestamp(file.expires_at);
      case 'nextRefreshAt':
        return renderTimestamp(file.next_refresh_at);
      case 'lastRefreshAt':
        return renderTimestamp(file.last_refresh_at);
      case 'consecutiveFailures':
        return <div className={styles.primary}>{file.consecutive_failures}</div>;
      case 'disabled':
        return file.disabled ? <span className={styles.disabled}>{t('common.yes')}</span> : <span className={styles.muted}>{t('common.no')}</span>;
      case 'error':
        return <div className={`${styles.error} ${styles.wrapAnywhere}`}>{file.last_error ?? t('common.notAvailable')}</div>;
      default:
        return null;
    }
  };

  return (
    <section className={styles.surface}>
      <div className={styles.header}>
        <div className={styles.headerCopy}>
          <div className={styles.title}>{t('files.title')}</div>
          <div className={styles.meta}>{t('files.meta')}</div>
        </div>
        <div className={`${styles.meta} ${styles.countMeta}`}>{t('files.visibleCount', { count: files.length })}</div>
      </div>
      {files.length === 0 ? (
        <div className={styles.empty}>{t('files.empty')}</div>
      ) : (
        <>
          <div className={styles.tableDesktop}>
            <div className={styles.tableWrap}>
              <table className={styles.table}>
                <thead>
                  <tr>
                    {COLUMNS.map((column) => (
                      <th key={column.key}>{t(column.labelKey)}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {files.map((file) => (
                    <tr key={file.file}>
                      {COLUMNS.map((column) => (
                        <td key={column.key}>{renderCell(file, column.key)}</td>
                      ))}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>

          <div className={styles.mobileCards}>
            {files.map((file) => (
              <article key={file.file} className={styles.mobileCard}>
                {COLUMNS.map((column) => (
                  <div key={column.key} className={styles.mobileField}>
                    <div className={styles.mobileFieldLabel}>{t(column.labelKey)}</div>
                    <div className={styles.mobileFieldValue}>{renderCell(file, column.key)}</div>
                  </div>
                ))}
              </article>
            ))}
          </div>
        </>
      )}
    </section>
  );
}
