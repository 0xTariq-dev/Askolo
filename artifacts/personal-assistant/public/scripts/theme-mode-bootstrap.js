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
  const systemMode = window.matchMedia('(prefers-color-scheme: dark)').matches
    ? 'dark'
    : 'light';
  const storedPreference = stored?.modePreference === 'system'
    ? systemMode
    : stored?.modePreference === 'light' || stored?.modePreference === 'dark'
      ? stored.modePreference
      : stored?.mode === 'light' || stored?.mode === 'dark'
        ? stored.mode
        : 'dark';
  const mode =
    queryMode === 'light' || queryMode === 'dark'
      ? queryMode
      : storedPreference;
  const allowedThemes = new Set(['blueHorizon', 'warmPaper', 'neutral']);
  const storedTheme = stored?.accountThemeId === 'neutralMaia'
    ? 'neutral'
    : stored?.accountThemeId;
  const theme = allowedThemes.has(storedTheme) ? storedTheme : 'neutral';
  const surface =
    stored?.surfaceStyles?.[mode] === 'tinted' ? 'tinted' : 'soft';

  root.classList.toggle('dark', mode === 'dark');
  root.style.colorScheme = mode;
  root.dataset.askoloTheme = theme;
  root.dataset.askoloMode = mode;
  root.dataset.askoloSurfaceStyle = surface;
  const appearance = {
    style: 'radix-maia',
    iconLibrary: 'tabler',
    menuColor: 'default-translucent',
    menuAccent: 'subtle',
    fontFamily: 'Outfit Variable, Cairo, sans-serif',
    radius: '1rem',
  };
  root.dataset.askoloStyle = appearance.style;
  root.dataset.askoloIconLibrary = appearance.iconLibrary;
  root.dataset.askoloMenuColor = appearance.menuColor;
  root.dataset.askoloMenuAccent = appearance.menuAccent;
  root.style.setProperty('--app-font-sans', appearance.fontFamily);
  root.style.setProperty('--app-font-display', appearance.fontFamily);
  root.style.setProperty('--radius', appearance.radius);
})();