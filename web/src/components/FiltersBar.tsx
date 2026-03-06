import { Search } from 'lucide-react';

import styles from './FiltersBar.module.scss';

export type FileFilter = 'all' | 'ok' | 'degraded' | 'reauth_required' | 'invalid_json';

const FILTERS: Array<{ key: FileFilter; label: string }> = [
  { key: 'all', label: 'All' },
  { key: 'ok', label: 'OK' },
  { key: 'degraded', label: 'Degraded' },
  { key: 'reauth_required', label: 'Reauth' },
  { key: 'invalid_json', label: 'Invalid' },
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
            {item.label}
          </button>
        ))}
      </div>
      <div className={styles.row}>
        <label className={styles.search}>
          <Search size={18} />
          <input
            value={search}
            onChange={(event) => onSearchChange(event.target.value)}
            placeholder="Search file or account"
          />
        </label>
        <label className={styles.checkbox}>
          <input
            type="checkbox"
            checked={showDisabled}
            onChange={(event) => onShowDisabledChange(event.target.checked)}
          />
          Show disabled files
        </label>
      </div>
    </div>
  );
}
