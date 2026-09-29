import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react';
import { setupI18n } from '@lingui/core';
import { I18nProvider } from '@lingui/react';
import { messages as englishMessages } from '../../locales/en/messages.po';
import { messages as arabicMessages } from '../../locales/ar/messages.po';
import {
  directionForLocale,
  formatDate,
  formatNumber,
  formatRelativeDate,
  getStoredLocale,
  isLocale,
  LOCALE_STORAGE_KEY,
  mergeCatalogWithFallback,
  type Locale,
  type TranslationValues,
} from '@/lib/locale';
import { messages, type MessageKey } from '@/lib/messages';

type LocaleContextValue = {
  locale: Locale;
  direction: 'ltr' | 'rtl';
  setLocale: (locale: Locale) => Promise<void>;
  t: (key: string, values?: TranslationValues) => string;
  plural: (key: string, count: number, values?: TranslationValues) => string;
  formatNumber: (value: number, options?: Intl.NumberFormatOptions) => string;
  formatDate: (value: Date | number, options?: Intl.DateTimeFormatOptions) => string;
  formatRelativeDate: (value: Date | number, now?: Date | number) => string;
};

const LocaleContext = createContext<LocaleContextValue | null>(null);

function applyDocumentLocale(locale: Locale) {
  const root = document.documentElement;
  root.lang = locale;
  root.dir = directionForLocale(locale);
}

function formatTranslationValues(
  values: TranslationValues,
  locale: Locale,
): TranslationValues {
  const formatted: TranslationValues = {};
  for (const [key, value] of Object.entries(values)) {
    formatted[key] = typeof value === 'number' ? formatNumber(value, locale) : value;
  }
  return formatted;
}

function translate(
  i18n: ReturnType<typeof setupI18n>,
  locale: Locale,
  key: string,
  values: TranslationValues = {},
): string {
  const descriptor = messages[key as MessageKey];
  if (!descriptor) return key;
  return i18n._(descriptor.id, formatTranslationValues(values, locale));
}

function translatePlural(
  i18n: ReturnType<typeof setupI18n>,
  locale: Locale,
  key: string,
  count: number,
  values: TranslationValues = {},
): string {
  const descriptor = messages[key as MessageKey];
  if (!descriptor) return key;
  return i18n._(descriptor.id, {
    ...formatTranslationValues(values, locale),
    count,
    formattedCount: formatNumber(count, locale),
  });
}

type LocaleProviderProps = {
  children: ReactNode;
  initialLocale?: Locale | null;
  persistAccountLocale?: (locale: Locale) => Promise<void>;
};

export function LocaleProvider({
  children,
  initialLocale,
  persistAccountLocale,
}: LocaleProviderProps) {
  const persistenceQueue = useRef<Promise<void>>(Promise.resolve());
  const [locale, setLocaleState] = useState<Locale>(() => {
    const selectedLocale = isLocale(initialLocale) ? initialLocale : getStoredLocale();
    applyDocumentLocale(selectedLocale);
    return selectedLocale;
  });
  const [i18n] = useState(() => {
    const instance = setupI18n();
    instance.load({
      en: englishMessages,
      ar: mergeCatalogWithFallback(englishMessages, arabicMessages),
    });
    instance.activate(locale);
    return instance;
  });

  const queueAccountLocale = useCallback((nextLocale: Locale) => {
    if (!persistAccountLocale) return Promise.resolve();
    const pending = persistenceQueue.current
      .catch(() => undefined)
      .then(() => persistAccountLocale(nextLocale));
    persistenceQueue.current = pending.then(
      () => undefined,
      () => undefined,
    );
    return pending;
  }, [persistAccountLocale]);

  const setLocale = useCallback((nextLocale: Locale) => {
    if (nextLocale === locale) return Promise.resolve();
    applyDocumentLocale(nextLocale);
    i18n.activate(nextLocale);
    try {
      window.localStorage.setItem(LOCALE_STORAGE_KEY, nextLocale);
    } catch {
      console.warn('Askolo could not save the language preference.');
    }
    setLocaleState(nextLocale);
    return queueAccountLocale(nextLocale);
  }, [i18n, locale, queueAccountLocale]);

  useEffect(() => {
    try {
      window.localStorage.setItem(LOCALE_STORAGE_KEY, locale);
    } catch {
      console.warn('Askolo could not save the language preference.');
    }
  }, [locale]);

  useEffect(() => {
    if (!persistAccountLocale || isLocale(initialLocale)) return;
    void queueAccountLocale(locale).catch(() => {
      console.warn('Askolo could not sync the account language preference.');
    });
  }, [initialLocale, locale, persistAccountLocale, queueAccountLocale]);

  useEffect(() => {
    const handleStorage = (event: StorageEvent) => {
      if (event.key !== LOCALE_STORAGE_KEY) return;
      const nextLocale = isLocale(event.newValue) ? event.newValue : 'en';
      applyDocumentLocale(nextLocale);
      i18n.activate(nextLocale);
      setLocaleState(nextLocale);
    };
    window.addEventListener('storage', handleStorage);
    return () => window.removeEventListener('storage', handleStorage);
  }, [i18n]);

  const value = useMemo<LocaleContextValue>(() => ({
    locale,
    direction: directionForLocale(locale),
    setLocale,
    t: (key, values) => translate(i18n, locale, key, values),
    plural: (key, count, values) => translatePlural(i18n, locale, key, count, values),
    formatNumber: (number, options) => formatNumber(number, locale, options),
    formatDate: (date, options) => formatDate(date, locale, options),
    formatRelativeDate: (date, now) => formatRelativeDate(date, locale, now),
  }), [i18n, locale, setLocale]);

  return (
    <I18nProvider i18n={i18n}>
      <LocaleContext.Provider value={value}>{children}</LocaleContext.Provider>
    </I18nProvider>
  );
}

export function useLocale(): LocaleContextValue {
  const context = useContext(LocaleContext);
  if (!context) throw new Error('useLocale must be used within LocaleProvider.');
  return context;
}