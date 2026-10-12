import {
  DEFAULT_THEME_PREFERENCES,
  isSurfaceStyleId,
  isThemeId,
  isThemeMode,
  type ResolvedThemePreferences,
  type ThemeId,
  type ThemeMode,
  type ThemeModePreference,
} from '@/lib/theme';

export const THEME_STORAGE_KEY = 'askolo-theme-preferences';
export const THEME_QUERY_PARAMETER = 'theme';

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function normalizeThemeId(value: unknown): ThemeId | null {
  if (value === 'neutralMaia') return 'neutral';
  return isThemeId(value) ? value : null;
}

export function loadThemePreferences(): ResolvedThemePreferences {
  const defaults = DEFAULT_THEME_PREFERENCES;
  let stored: Record<string, unknown> = {};
  let queryMode: string | null = null;

  if (typeof window !== 'undefined') {
    queryMode = new URLSearchParams(window.location.search).get(THEME_QUERY_PARAMETER);

    try {
      const serialized = window.localStorage.getItem(THEME_STORAGE_KEY);
      const parsed: unknown = serialized ? JSON.parse(serialized) : undefined;
      if (isRecord(parsed)) stored = parsed;
    } catch {
      console.warn('Askolo could not read saved theme preferences.');
    }
  }

  const rawSurfaceStyles = isRecord(stored.surfaceStyles) ? stored.surfaceStyles : {};
  const queryChoice = isThemeMode(queryMode) ? queryMode : null;
  const storedChoice = stored.modePreference === 'system' || isThemeMode(stored.modePreference)
    ? stored.modePreference as ThemeModePreference
    : isThemeMode(stored.mode) ? stored.mode : defaults.modePreference;
  const modePreference = queryChoice ?? storedChoice;
  let mode: ThemeMode = modePreference === 'system' && typeof window !== 'undefined'
    ? (window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light')
    : modePreference === 'system' ? defaults.mode : modePreference;

  const rawSpaceOverrides = isRecord(stored.spaceOverrides) ? stored.spaceOverrides : {};
  const spaceOverrides = Object.create(null) as Record<string, ThemeId>;
  for (const [spaceId, themeId] of Object.entries(rawSpaceOverrides)) {
    const normalizedThemeId = normalizeThemeId(themeId);
    if (spaceId.trim() && spaceId.length <= 128 && normalizedThemeId) {
      spaceOverrides[spaceId] = normalizedThemeId;
    }
  }

  return {
    accountThemeId: normalizeThemeId(stored.accountThemeId) ?? defaults.accountThemeId,
    mode,
    modePreference,
    surfaceStyles: {
      light: isSurfaceStyleId(rawSurfaceStyles.light)
        ? rawSurfaceStyles.light
        : defaults.surfaceStyles.light,
      dark: isSurfaceStyleId(rawSurfaceStyles.dark)
        ? rawSurfaceStyles.dark
        : defaults.surfaceStyles.dark,
    },
    spaceOverrides,
  };
}