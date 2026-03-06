import { AlertTriangle, Info } from 'lucide-react';

import styles from './StatusBanner.module.scss';

export function StatusBanner({
  title,
  message,
  variant = 'info',
}: {
  title: string;
  message: string;
  variant?: 'info' | 'warning';
}) {
  const Icon = variant === 'warning' ? AlertTriangle : Info;
  return (
    <div className={`${styles.banner} ${variant === 'warning' ? styles.warning : styles.info}`}>
      <Icon size={20} />
      <div>
        <div className={styles.title}>{title}</div>
        <div className={styles.message}>{message}</div>
      </div>
    </div>
  );
}
