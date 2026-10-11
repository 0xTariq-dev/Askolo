(() => {
  const root = document.documentElement;
  let stored = null;

  try {
    const serialized = window.localStorage.getItem('askolo-theme-preferences');
    stored = serialized ? JSON.parse(serialized) : null;
  } catch {
    // The app's theme provider logs storage errors and applies its safe default.
  }

  const queryMode = new URLSearchParams(window.location.search).get('theme');
  const mode =
    queryMode === 'light' || queryMode === 'dark'
      ? queryMode
      : stored?.mode === 'light' || stored?.mode === 'dark'
        ? stored.mode
        : 'dark';
  const allowedThemes = new Set(['blueHorizon', 'warmPaper', 'neutralMaia']);
  const theme = allowedThemes.has(stored?.accountThemeId)
    ? stored.accountThemeId
    : 'neutralMaia';
  const surface =
    stored?.surfaceStyles?.[mode] === 'tinted' ? 'tinted' : 'soft';

  root.classList.toggle('dark', mode === 'dark');
  root.style.colorScheme = mode;
  root.dataset.askoloTheme = theme;
  root.dataset.askoloMode = mode;
  root.dataset.askoloSurfaceStyle = surface;
  const appearance = theme === 'neutralMaia'
    ? {
        style: 'radix-maia',
        iconLibrary: 'tabler',
        menuColor: 'default-translucent',
        menuAccent: 'subtle',
        fontFamily: 'Outfit Variable, Cairo, sans-serif',
        radius: '1rem',
      }
    : {
        style: 'askolo',
        iconLibrary: 'lucide',
        menuColor: 'default',
        menuAccent: 'subtle',
        fontFamily: 'Cairo, sans-serif',
        radius: '0.875rem',
      };
  root.dataset.askoloStyle = appearance.style;
  root.dataset.askoloIconLibrary = appearance.iconLibrary;
  root.dataset.askoloMenuColor = appearance.menuColor;
  root.dataset.askoloMenuAccent = appearance.menuAccent;
  root.style.setProperty('--app-font-sans', appearance.fontFamily);
  root.style.setProperty('--app-font-display', appearance.fontFamily);
  root.style.setProperty('--radius', appearance.radius);
})();