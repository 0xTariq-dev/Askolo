import {
  DEFAULT_THEME_PREFERENCES,
  isSurfaceStyleId,
  isThemeId,
  isThemeMode,
  type ResolvedThemePreferences,
  type ThemeId,
} from '@/lib/theme';

export const THEME_STORAGE_KEY = 'askolo-theme-preferences';
export const THEME_QUERY_PARAMETER = 'theme';

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
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
  const rawSpaceOverrides = isRecord(stored.spaceOverrides) ? stored.spaceOverrides : {};
  const spaceOverrides = Object.create(null) as Record<string, ThemeId>;
  for (const [spaceId, themeId] of Object.entries(rawSpaceOverrides)) {
    if (spaceId.trim() && spaceId.length <= 128 && isThemeId(themeId)) {
      spaceOverrides[spaceId] = themeId;
    }
  }

  return {
    accountThemeId: isThemeId(stored.accountThemeId)
      ? stored.accountThemeId
      : defaults.accountThemeId,
    mode: isThemeMode(queryMode)
      ? queryMode
      : isThemeMode(stored.mode)
        ? stored.mode
        : defaults.mode,
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