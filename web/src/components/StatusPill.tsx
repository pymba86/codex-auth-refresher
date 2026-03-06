import styles from './StatusPill.module.scss';

const LABELS: Record<string, string> = {
  ok: 'OK',
  degraded: 'Проблема',
  reauth_required: 'Нужен вход',
  invalid_json: 'Некорректный JSON',
};

export function StatusPill({ state }: { state: string }) {
  const variant = styles[state] ?? styles.default;
  return (
    <span className={`${styles.pill} ${variant}`}>
      <span className={styles.dot} />
      {LABELS[state] ?? state}
    </span>
  );
}
