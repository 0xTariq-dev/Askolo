(() => {
  const root = document.documentElement;
  let storedMode = null;

  try {
    const serialized = window.localStorage.getItem('askolo-theme-preferences');
    const stored = serialized ? JSON.parse(serialized) : null;
    if (stored?.mode === 'light' || stored?.mode === 'dark') {
      storedMode = stored.mode;
    }
  } catch {
    // The app's theme provider logs storage errors and applies its safe default.
  }

  const queryMode = new URLSearchParams(window.location.search).get('theme');
  const mode =
    queryMode === 'light' || queryMode === 'dark'
      ? queryMode
      : storedMode ?? 'light';

  root.classList.toggle('dark', mode === 'dark');
  root.style.colorScheme = mode;
})();