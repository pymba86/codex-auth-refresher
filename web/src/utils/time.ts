const relativeFormatter = new Intl.RelativeTimeFormat('ru-RU', { numeric: 'auto' });
const absoluteFormatter = new Intl.DateTimeFormat('ru-RU', {
  dateStyle: 'medium',
  timeStyle: 'short',
  timeZone: 'UTC',
});

const UNITS: Array<[Intl.RelativeTimeFormatUnit, number]> = [
  ['day', 86_400],
  ['hour', 3_600],
  ['minute', 60],
  ['second', 1],
];

export function formatAbsolute(value?: string | null): string {
  if (!value) {
    return '—';
  }

  return `${absoluteFormatter.format(new Date(value))} UTC`;
}

export function formatRelative(value?: string | null, now = Date.now()): string {
  if (!value) {
    return '—';
  }

  const diffSeconds = Math.round((new Date(value).getTime() - now) / 1000);
  for (const [unit, divisor] of UNITS) {
    if (Math.abs(diffSeconds) >= divisor || unit === 'second') {
      return relativeFormatter.format(Math.round(diffSeconds / divisor), unit);
    }
  }
  return '—';
}

export function formatDuration(totalSeconds: number): string {
  const days = Math.floor(totalSeconds / 86_400);
  const hours = Math.floor((totalSeconds % 86_400) / 3_600);
  const minutes = Math.floor((totalSeconds % 3_600) / 60);

  const parts: string[] = [];
  if (days > 0) {
    parts.push(`${days} д`);
  }
  if (hours > 0 || days > 0) {
    parts.push(`${hours} ч`);
  }
  parts.push(`${minutes} мин`);
  return parts.join(' ');
}
