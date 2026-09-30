import { getThemeVariables } from '@/lib/theme';
import { loadThemePreferences } from '@/lib/theme-preferences';

const preferences = loadThemePreferences();
const root = document.documentElement;
const surfaceStyle = preferences.surfaceStyles[preferences.mode];

root.classList.toggle('dark', preferences.mode === 'dark');
root.style.colorScheme = preferences.mode;
root.dataset.askoloTheme = preferences.accountThemeId;
root.dataset.askoloSurfaceStyle = surfaceStyle;

for (const [name, value] of Object.entries(
  getThemeVariables(preferences.accountThemeId, preferences.mode, surfaceStyle),
)) {
  if (typeof value === 'string') root.style.setProperty(name, value);
}