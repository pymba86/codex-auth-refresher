import type { Locale } from '../i18n';

const LOCALE_CODES: Record<Locale, string> = {
  ru: 'ru-RU',
  en: 'en-US',
};

const relativeFormatters = new Map<Locale, Intl.RelativeTimeFormat>();
const absoluteFormatters = new Map<Locale, Intl.DateTimeFormat>();
const clockFormatters = new Map<Locale, Intl.DateTimeFormat>();

const UNITS: Array<[Intl.RelativeTimeFormatUnit, number]> = [
  ['day', 86_400],
  ['hour', 3_600],
  ['minute', 60],
  ['second', 1],
];

function parseDate(value?: Date | string | null): Date | null {
  if (!value) {
    return null;
  }

  const date = value instanceof Date ? value : new Date(value);
  return Number.isNaN(date.getTime()) ? null : date;
}

function getRelativeFormatter(locale: Locale): Intl.RelativeTimeFormat {
  const existing = relativeFormatters.get(locale);
  if (existing) {
    return existing;
  }

  const formatter = new Intl.RelativeTimeFormat(LOCALE_CODES[locale], { numeric: 'auto' });
  relativeFormatters.set(locale, formatter);
  return formatter;
}

function getAbsoluteFormatter(locale: Locale): Intl.DateTimeFormat {
  const existing = absoluteFormatters.get(locale);
  if (existing) {
    return existing;
  }

  const formatter = new Intl.DateTimeFormat(LOCALE_CODES[locale], {
    dateStyle: 'medium',
    timeStyle: 'short',
    timeZone: 'UTC',
  });
  absoluteFormatters.set(locale, formatter);
  return formatter;
}

function getClockFormatter(locale: Locale): Intl.DateTimeFormat {
  const existing = clockFormatters.get(locale);
  if (existing) {
    return existing;
  }

  const formatter = new Intl.DateTimeFormat(LOCALE_CODES[locale], {
    timeStyle: 'medium',
  });
  clockFormatters.set(locale, formatter);
  return formatter;
}

function formatCompactDurationUnit(value: number, unit: 'd' | 'h' | 'm', locale: Locale): string {
  if (locale === 'ru') {
    const ruUnit = unit === 'd' ? 'д' : unit === 'h' ? 'ч' : 'мин';
    return `${value} ${ruUnit}`;
  }
  return `${value}${unit}`;
}

export function formatAbsolute(value: Date | string | null | undefined, locale: Locale): string {
  const date = parseDate(value);
  if (!date) {
    return '—';
  }

  return `${getAbsoluteFormatter(locale).format(date)} UTC`;
}

export function formatClockTime(value: Date | string | null | undefined, locale: Locale): string {
  const date = parseDate(value);
  if (!date) {
    return '—';
  }

  return getClockFormatter(locale).format(date);
}

export function formatRelative(value: Date | string | null | undefined, locale: Locale, now = Date.now()): string {
  const date = parseDate(value);
  if (!date) {
    return '—';
  }

  const diffSeconds = Math.round((date.getTime() - now) / 1000);
  const formatter = getRelativeFormatter(locale);

  for (const [unit, divisor] of UNITS) {
    if (Math.abs(diffSeconds) >= divisor || unit === 'second') {
      return formatter.format(Math.round(diffSeconds / divisor), unit);
    }
  }
  return '—';
}

export function formatDuration(totalSeconds: number, locale: Locale): string {
  const normalized = Math.max(0, Math.floor(totalSeconds));
  const days = Math.floor(normalized / 86_400);
  const hours = Math.floor((normalized % 86_400) / 3_600);
  const minutes = Math.floor((normalized % 3_600) / 60);

  const parts: string[] = [];
  if (days > 0) {
    parts.push(formatCompactDurationUnit(days, 'd', locale));
  }
  if (hours > 0 || days > 0) {
    parts.push(formatCompactDurationUnit(hours, 'h', locale));
  }
  parts.push(formatCompactDurationUnit(minutes, 'm', locale));
  return parts.join(' ');
}
