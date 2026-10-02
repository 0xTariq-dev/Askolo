(() => {
  const root = document.documentElement;
  const registry = window.__ASKOLO_LOCALE_BOOTSTRAP__;
  if (!registry || !registry.locales || !registry.locales[registry.defaultLocale]) return;

  let locale = registry.defaultLocale;

  try {
    const storedLocale = window.localStorage.getItem('askolo-locale');
    if (Object.prototype.hasOwnProperty.call(registry.locales, storedLocale)) {
      locale = storedLocale;
    }
  } catch {
    // The app locale provider uses English when browser storage is unavailable.
  }

  root.lang = locale;
  root.dir = registry.locales[locale].direction;
})();