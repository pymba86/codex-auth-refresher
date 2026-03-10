import { useI18n } from '../i18n';

import styles from './StatusPill.module.scss';

const LABEL_KEYS = {
  ok: 'state.ok',
  degraded: 'state.degraded',
  reauth_required: 'state.reauth_required',
  invalid_json: 'state.invalid_json',
} as const;

type KnownState = keyof typeof LABEL_KEYS;

function isKnownState(state: string): state is KnownState {
  return state in LABEL_KEYS;
}

export function StatusPill({ state }: { state: string }) {
  const { t } = useI18n();
  const variant = styles[state] ?? styles.default;
  return (
    <span className={`${styles.pill} ${variant}`}>
      <span className={styles.dot} />
      {isKnownState(state) ? t(LABEL_KEYS[state]) : state}
    </span>
  );
}
