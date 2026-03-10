import { Search } from 'lucide-react';

import { type TranslationKey, useI18n } from '../i18n';
import styles from './FiltersBar.module.scss';

export type FileFilter = 'all' | 'ok' | 'degraded' | 'reauth_required' | 'invalid_json';

const FILTERS: Array<{ key: FileFilter; labelKey: TranslationKey }> = [
  { key: 'all', labelKey: 'filters.all' },
  { key: 'ok', labelKey: 'filters.ok' },
  { key: 'degraded', labelKey: 'filters.degraded' },
  { key: 'reauth_required', labelKey: 'filters.reauthRequired' },
  { key: 'invalid_json', labelKey: 'filters.invalidJson' },
];

export function FiltersBar({
  filter,
  onFilterChange,
  search,
  onSearchChange,
  showDisabled,
  onShowDisabledChange,
}: {
  filter: FileFilter;
  onFilterChange: (value: FileFilter) => void;
  search: string;
  onSearchChange: (value: string) => void;
  showDisabled: boolean;
  onShowDisabledChange: (value: boolean) => void;
}) {
  const { t } = useI18n();

  return (
    <div className={styles.toolbar}>
      <div className={styles.chips}>
        {FILTERS.map((item) => (
          <button
            key={item.key}
            type="button"
            className={`${styles.chip} ${filter === item.key ? styles.active : ''}`}
            onClick={() => onFilterChange(item.key)}
          >
            {t(item.labelKey)}
          </button>
        ))}
      </div>
      <div className={styles.row}>
        <label className={styles.search}>
          <Search size={18} />
          <input
            value={search}
            onChange={(event) => onSearchChange(event.target.value)}
            placeholder={t('filters.searchPlaceholder')}
          />
        </label>
        <label className={styles.checkbox}>
          <input
            type="checkbox"
            checked={showDisabled}
            onChange={(event) => onShowDisabledChange(event.target.checked)}
          />
          {t('filters.showDisabled')}
        </label>
      </div>
    </div>
  );
}
