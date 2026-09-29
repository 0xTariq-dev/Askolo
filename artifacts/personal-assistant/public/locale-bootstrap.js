(() => {
  const root = document.documentElement;
  let locale = 'en';

  try {
    const storedLocale = window.localStorage.getItem('askolo-locale');
    if (storedLocale === 'en' || storedLocale === 'ar') locale = storedLocale;
  } catch {
    // The app locale provider uses English when browser storage is unavailable.
  }

  root.lang = locale;
  root.dir = locale === 'ar' ? 'rtl' : 'ltr';
})();