import { arSA, enUS } from 'date-fns/locale';
import localeRegistryData from './locale-registry.json';

export const LOCALE_STORAGE_KEY = 'askolo-locale';

export const DEFAULT_LOCALE = localeRegistryData.defaultLocale as keyof typeof localeRegistryData.locales;
export const SUPPORTED_LOCALES = Object.keys(localeRegistryData.locales) as Array<
  keyof typeof localeRegistryData.locales
>;
export type Locale = (typeof SUPPORTED_LOCALES)[number];
export type TextDirection = 'ltr' | 'rtl';
export type TranslationValues = Record<string, string | number>;
export type CalendarWeekStart = 0 | 1 | 2 | 3 | 4 | 5 | 6;
export type DateFnsLocale = import('date-fns').Locale;
export type MessageCatalog = Record<string, unknown>;
export type CatalogLoader = () => Promise<MessageCatalog>;

const localeMetadata = localeRegistryData.locales;
const dateFnsLocales: Record<Locale, DateFnsLocale> = { en: enUS, ar: arSA };
const catalogModules = typeof window !== 'undefined'
  ? import.meta.glob<MessageCatalog>('../../locales/*/messages.po', {
      import: 'messages',
    })
  : {};

type LocaleRegistryEntry = Omit<
  (typeof localeMetadata)[Locale],
  'dateFnsLocale' | 'weekStartsOn'
> & {
  dateFnsLocaleCode: string;
  weekStartsOn: CalendarWeekStart;
  dateFnsLocale: DateFnsLocale;
  loadCatalog: CatalogLoader;
};

export const LOCALE_REGISTRY: Record<Locale, LocaleRegistryEntry> = Object.fromEntries(
  SUPPORTED_LOCALES.map((locale) => {
    const metadata = localeMetadata[locale];
    const loader = catalogModules[metadata.catalogModule];
    return [
      locale,
      {
        ...metadata,
        dateFnsLocaleCode: metadata.dateFnsLocale,
        weekStartsOn: metadata.weekStartsOn as CalendarWeekStart,
        dateFnsLocale: dateFnsLocales[locale],
        loadCatalog: async () => {
          if (!loader) {
            throw new Error(`No Vite catalog chunk is registered for locale "${locale}".`);
          }
          return loader();
        },
      },
    ];
  }),
) as Record<Locale, LocaleRegistryEntry>;

export function mergeCatalogWithFallback<T extends Record<string, unknown>>(
  fallback: T,
  localized: Partial<T>,
): T {
  return { ...fallback, ...localized };
}

export function isLocaleCode(value: unknown): value is string {
  return typeof value === 'string' && /^[a-z]{2}$/.test(value);
}

export function isLocale(value: unknown): value is Locale {
  return isLocaleCode(value) && Object.hasOwn(localeMetadata, value);
}

export function directionForLocale(locale: Locale): TextDirection {
  return LOCALE_REGISTRY[locale].direction as TextDirection;
}

export async function loadLocaleCatalog(
  locale: Locale,
  loaders: Partial<Record<Locale, CatalogLoader>> = Object.fromEntries(
    SUPPORTED_LOCALES.map((code) => [code, LOCALE_REGISTRY[code].loadCatalog]),
  ),
): Promise<MessageCatalog> {
  const loadEnglish = loaders[DEFAULT_LOCALE];
  if (!loadEnglish) {
    throw new Error(`The default "${DEFAULT_LOCALE}" catalog loader is not registered.`);
  }

  const fallback = await loadEnglish();
  if (locale === DEFAULT_LOCALE) return fallback;

  try {
    const loadLocalized = loaders[locale];
    if (!loadLocalized) {
      throw new Error(`No catalog loader is registered for locale "${locale}".`);
    }
    return mergeCatalogWithFallback(fallback, await loadLocalized());
  } catch (error) {
    console.warn(`Askolo could not load the "${locale}" catalog; using English instead.`, error);
    return fallback;
  }
}

export function getStoredLocale(
  storage: Pick<Storage, 'getItem'> | null = getLocalStorage(),
): Locale {
  try {
    const stored = storage?.getItem(LOCALE_STORAGE_KEY);
    return isLocale(stored) ? stored : DEFAULT_LOCALE;
  } catch {
    return DEFAULT_LOCALE;
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
  return new Intl.NumberFormat(LOCALE_REGISTRY[locale].numberFormatLocale, options).format(value);
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
  const metadata = LOCALE_REGISTRY[locale];
  return new Intl.DateTimeFormat(metadata.dateFormatLocale, {
    ...options,
    calendar: metadata.calendar,
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
  return new Intl.RelativeTimeFormat(LOCALE_REGISTRY[locale].numberFormatLocale, {
    numeric: 'auto',
  }).format(Math.round(seconds / unitSeconds), unit);
}