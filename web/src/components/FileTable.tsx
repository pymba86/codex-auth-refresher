import type { DashboardFile } from '../types';
import { formatAbsolute, formatRelative } from '../utils/time';

import { StatusPill } from './StatusPill';
import styles from './FileTable.module.scss';

export function FileTable({ files }: { files: DashboardFile[] }) {
  return (
    <section className={styles.surface}>
      <div className={styles.header}>
        <div>
          <div className={styles.title}>Отслеживаемые auth-файлы</div>
          <div className={styles.meta}>Текущее состояние refresh для каждого Codex auth-документа.</div>
        </div>
        <div className={styles.meta}>Видимых записей: {files.length}</div>
      </div>
      {files.length === 0 ? (
        <div className={styles.empty}>По текущим фильтрам auth-файлы не найдены.</div>
      ) : (
        <div className={styles.tableWrap}>
          <table className={styles.table}>
            <thead>
              <tr>
                <th>Файл</th>
                <th>Аккаунт</th>
                <th>Схема</th>
                <th>Состояние</th>
                <th>Истекает</th>
                <th>Следующий refresh</th>
                <th>Последний refresh</th>
                <th>Ошибки подряд</th>
                <th>Отключён</th>
                <th>Ошибка</th>
              </tr>
            </thead>
            <tbody>
              {files.map((file) => (
                <tr key={file.file}>
                  <td>
                    <div className={styles.primary}>{file.file}</div>
                  </td>
                  <td>
                    <div className={styles.primary}>{file.account_id ?? '—'}</div>
                  </td>
                  <td>
                    <div className={styles.primary}>{file.schema ?? '—'}</div>
                  </td>
                  <td>
                    <StatusPill state={file.state} />
                  </td>
                  <td title={file.expires_at}>
                    <div className={styles.primary}>{formatRelative(file.expires_at)}</div>
                    <div className={styles.subtle}>{formatAbsolute(file.expires_at)}</div>
                  </td>
                  <td title={file.next_refresh_at}>
                    <div className={styles.primary}>{formatRelative(file.next_refresh_at)}</div>
                    <div className={styles.subtle}>{formatAbsolute(file.next_refresh_at)}</div>
                  </td>
                  <td title={file.last_refresh_at}>
                    <div className={styles.primary}>{formatRelative(file.last_refresh_at)}</div>
                    <div className={styles.subtle}>{formatAbsolute(file.last_refresh_at)}</div>
                  </td>
                  <td>
                    <div className={styles.primary}>{file.consecutive_failures}</div>
                  </td>
                  <td>
                    {file.disabled ? <span className={styles.disabled}>Да</span> : <span className={styles.muted}>Нет</span>}
                  </td>
                  <td>
                    <div className={styles.error}>{file.last_error ?? '—'}</div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
