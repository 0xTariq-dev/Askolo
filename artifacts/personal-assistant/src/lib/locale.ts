export const LOCALE_STORAGE_KEY = 'askolo-locale';

export const SUPPORTED_LOCALES = ['en', 'ar'] as const;
export type Locale = (typeof SUPPORTED_LOCALES)[number];
export type TextDirection = 'ltr' | 'rtl';
export type TranslationValues = Record<string, string | number>;

export function mergeCatalogWithFallback<T extends Record<string, unknown>>(
  fallback: T,
  localized: Partial<T>,
): T {
  return { ...fallback, ...localized };
}

function formattingLocale(locale: Locale): string {
  return locale === 'ar' ? 'ar-u-nu-arab' : locale;
}

export function isLocale(value: unknown): value is Locale {
  return typeof value === 'string' && SUPPORTED_LOCALES.includes(value as Locale);
}

export function directionForLocale(locale: Locale): TextDirection {
  return locale === 'ar' ? 'rtl' : 'ltr';
}

export function getStoredLocale(
  storage: Pick<Storage, 'getItem'> | null = getLocalStorage(),
): Locale {
  try {
    const stored = storage?.getItem(LOCALE_STORAGE_KEY);
    return isLocale(stored) ? stored : 'en';
  } catch {
    return 'en';
  }
}

function getLocalStorage(): Pick<Storage, 'getItem'> | null {
  if (typeof window === 'undefined') return null;
  try {
    return window.localStorage;
  } catch {
    return null;
  }
}

export function formatNumber(
  value: number,
  locale: Locale,
  options?: Intl.NumberFormatOptions,
): string {
  return new Intl.NumberFormat(formattingLocale(locale), options).format(value);
}

export function formatDate(
  value: Date | number,
  locale: Locale,
  options: Intl.DateTimeFormatOptions = {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  },
): string {
  return new Intl.DateTimeFormat(formattingLocale(locale), {
    ...options,
    calendar: 'gregory',
  }).format(value);
}

export function formatRelativeDate(
  value: Date | number,
  locale: Locale,
  now: Date | number = Date.now(),
): string {
  const targetTime = value instanceof Date ? value.getTime() : value;
  const currentTime = now instanceof Date ? now.getTime() : now;
  const seconds = (targetTime - currentTime) / 1000;
  const intervals: Array<[Intl.RelativeTimeFormatUnit, number]> = [
    ['year', 31_536_000],
    ['month', 2_592_000],
    ['week', 604_800],
    ['day', 86_400],
    ['hour', 3_600],
    ['minute', 60],
    ['second', 1],
  ];
  const [unit, unitSeconds] =
    intervals.find(([, size]) => Math.abs(seconds) >= size) ?? ['second', 1];
  return new Intl.RelativeTimeFormat(formattingLocale(locale), {
    numeric: 'auto',
  }).format(Math.round(seconds / unitSeconds), unit);
}