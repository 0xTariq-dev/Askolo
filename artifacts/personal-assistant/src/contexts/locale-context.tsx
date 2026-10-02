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
import {
  DEFAULT_LOCALE,
  directionForLocale,
  formatDate,
  formatNumber,
  formatRelativeDate,
  getStoredLocale,
  isLocale,
  loadLocaleCatalog,
  LOCALE_STORAGE_KEY,
  LOCALE_REGISTRY,
  type Locale,
  type MessageCatalog,
  type TranslationValues,
  type DateFnsLocale,
  type CalendarWeekStart,
} from '@/lib/locale';
import { messages, type MessageKey } from '@/lib/messages';

type LocaleContextValue = {
  locale: Locale;
  direction: 'ltr' | 'rtl';
  dateFnsLocale: DateFnsLocale;
  weekStartsOn: CalendarWeekStart;
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

function loadMessages(i18n: ReturnType<typeof setupI18n>, locale: Locale, catalog: MessageCatalog) {
  i18n.load(locale, catalog as never);
  i18n.activate(locale);
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
  const catalogPromises = useRef<Partial<Record<Locale, Promise<MessageCatalog>>>>({});
  const localeChangeSequence = useRef(0);
  const [catalogReadyLocale, setCatalogReadyLocale] = useState<Locale | null>(null);
  const [catalogLoadFailed, setCatalogLoadFailed] = useState(false);
  const [locale, setLocaleState] = useState<Locale>(() => {
    const selectedLocale = isLocale(initialLocale) ? initialLocale : getStoredLocale();
    applyDocumentLocale(selectedLocale);
    return selectedLocale;
  });
  const [i18n] = useState(() => {
    const instance = setupI18n();
    instance.load(locale, {});
    instance.activate(locale);
    return instance;
  });

  const getCatalog = useCallback((requestedLocale: Locale) => {
    const cached = catalogPromises.current[requestedLocale];
    if (cached) return cached;
    const pending = loadLocaleCatalog(requestedLocale);
    catalogPromises.current[requestedLocale] = pending;
    void pending.catch(() => {
      delete catalogPromises.current[requestedLocale];
    });
    return pending;
  }, []);

  useEffect(() => {
    let active = true;
    setCatalogLoadFailed(false);
    void getCatalog(locale)
      .then((catalog) => {
        if (active) {
          loadMessages(i18n, locale, catalog);
          setCatalogReadyLocale(locale);
        }
      })
      .catch((error: unknown) => {
        if (active) {
          setCatalogLoadFailed(true);
          console.error('Askolo could not load its default English catalog.', error);
        }
      });
    return () => {
      active = false;
    };
  }, [getCatalog, i18n, locale]);

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

  const setLocale = useCallback(async (nextLocale: Locale) => {
    const sequence = ++localeChangeSequence.current;
    if (nextLocale === locale) return Promise.resolve();
    const catalog = await getCatalog(nextLocale);
    if (sequence !== localeChangeSequence.current) return;
    loadMessages(i18n, nextLocale, catalog);
    setCatalogReadyLocale(nextLocale);
    setCatalogLoadFailed(false);
    applyDocumentLocale(nextLocale);
    try {
      window.localStorage.setItem(LOCALE_STORAGE_KEY, nextLocale);
    } catch {
      console.warn('Askolo could not save the language preference.');
    }
    setLocaleState(nextLocale);
    await queueAccountLocale(nextLocale);
  }, [getCatalog, i18n, locale, queueAccountLocale]);

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
      localeChangeSequence.current += 1;
      const nextLocale = isLocale(event.newValue) ? event.newValue : DEFAULT_LOCALE;
      applyDocumentLocale(nextLocale);
      setLocaleState(nextLocale);
    };
    window.addEventListener('storage', handleStorage);
    return () => window.removeEventListener('storage', handleStorage);
  }, []);

  const value = useMemo<LocaleContextValue>(() => ({
    locale,
    direction: directionForLocale(locale),
    dateFnsLocale: LOCALE_REGISTRY[locale].dateFnsLocale,
    weekStartsOn: LOCALE_REGISTRY[locale].weekStartsOn,
    setLocale,
    t: (key, values) => translate(i18n, locale, key, values),
    plural: (key, count, values) => translatePlural(i18n, locale, key, count, values),
    formatNumber: (number, options) => formatNumber(number, locale, options),
    formatDate: (date, options) => formatDate(date, locale, options),
    formatRelativeDate: (date, now) => formatRelativeDate(date, locale, now),
  }), [i18n, locale, setLocale]);

  return (
    <I18nProvider i18n={i18n}>
      <LocaleContext.Provider value={value}>
        {catalogReadyLocale === locale
          ? children
          : catalogLoadFailed
            ? (
              <div
                className="flex min-h-screen items-center justify-center p-6 text-sm text-muted-foreground"
                role="alert"
              >
                Could not load language content. Refresh the page to try again.
              </div>
            )
            : <div className="min-h-screen" aria-busy="true" />}
      </LocaleContext.Provider>
    </I18nProvider>
  );
}

export function useLocale(): LocaleContextValue {
  const context = useContext(LocaleContext);
  if (!context) throw new Error('useLocale must be used within LocaleProvider.');
  return context;
}