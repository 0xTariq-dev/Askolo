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
  const theme =
    stored?.accountThemeId === 'warmPaper' ? 'warmPaper' : 'blueHorizon';
  const surface =
    stored?.surfaceStyles?.[mode] === 'tinted' ? 'tinted' : 'soft';

  root.classList.toggle('dark', mode === 'dark');
  root.style.colorScheme = mode;
  root.dataset.askoloTheme = theme;
  root.dataset.askoloMode = mode;
  root.dataset.askoloSurfaceStyle = surface;
})();