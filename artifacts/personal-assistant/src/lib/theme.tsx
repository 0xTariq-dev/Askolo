import { createContext, useContext, useEffect, useLayoutEffect, useMemo, type ReactNode } from 'react';
import designTokens from './askolo-design-tokens.json';

export type ThemeMode = 'light' | 'dark';
export type ThemeModePreference = ThemeMode | 'system';
export type ThemeId = 'blueHorizon' | 'warmPaper' | 'neutral';
export type SurfaceStyleId = 'soft' | 'tinted';

export interface ResolvedThemePreferences {
  accountThemeId: ThemeId;
  mode: ThemeMode;
  modePreference: ThemeModePreference;
  surfaceStyles: Record<ThemeMode, SurfaceStyleId>;
  spaceOverrides: Record<string, ThemeId>;
}

type TokenColor = Record<string, { $value: string }>;
interface SourceTheme {
  label: string;
  description?: string;
  appearance?: {
    style: string;
    radius: string;
    fontFamily: string[];
    iconLibrary: string;
    menuColor: string;
    menuAccent: string;
  };
  light: TokenColor;
  dark: TokenColor;
  surfaceOptions: Record<ThemeMode, Record<SurfaceStyleId, TokenColor>>;
}

const source = designTokens as unknown as {
  color: Record<ThemeMode, TokenColor>;
  themePresets: Record<ThemeId, SourceTheme>;
};

interface ThemeChartTokens {
  chart1: string;
  chart2: string;
  chart3: string;
  chart4: string;
  chart5: string;
  chart6: string;
  chart7: string;
  chart8: string;
}

interface ThemePreset {
  label: string;
  description: string;
  appearance: NonNullable<SourceTheme['appearance']>;
  light: ThemeChartTokens;
  dark: ThemeChartTokens;
}

function readHexColor(colors: TokenColor, role: string): string {
  const value = colors[role]?.$value;
  if (!value) throw new Error(`Askolo design token "${role}" is missing.`);
  return value;
}

function hexToHslChannels(hex: string): string {
  const match = /^#([0-9a-f]{6})$/i.exec(hex);
  if (!match) throw new Error(`Expected a six-digit Askolo color token, received "${hex}".`);

  const [red, green, blue] = match[1]
    .match(/.{2}/g)!
    .map((channel) => Number.parseInt(channel, 16) / 255);
  const max = Math.max(red, green, blue);
  const min = Math.min(red, green, blue);
  const lightness = (max + min) / 2;
  let hue = 0;
  let saturation = 0;

  if (max !== min) {
    const delta = max - min;
    saturation = lightness > 0.5 ? delta / (2 - max - min) : delta / (max + min);
    if (max === red) hue = (green - blue) / delta + (green < blue ? 6 : 0);
    else if (max === green) hue = (blue - red) / delta + 2;
    else hue = (red - green) / delta + 4;
    hue /= 6;
  }

  return `${Math.round(hue * 360)} ${Math.round(saturation * 1000) / 10}% ${Math.round(lightness * 1000) / 10}%`;
}

function chartTokens(themeId: ThemeId, mode: ThemeMode): ThemeChartTokens {
  const colors = source.themePresets[themeId][mode];
  return {
    chart1: hexToHslChannels(readHexColor(colors, 'chart1')),
    chart2: hexToHslChannels(readHexColor(colors, 'chart2')),
    chart3: hexToHslChannels(readHexColor(colors, 'chart3')),
    chart4: hexToHslChannels(readHexColor(colors, 'chart4')),
    chart5: hexToHslChannels(readHexColor(colors, 'chart5')),
    chart6: hexToHslChannels(readHexColor(colors, 'chart6')),
    chart7: hexToHslChannels(readHexColor(colors, 'chart7')),
    chart8: hexToHslChannels(readHexColor(colors, 'chart8')),
  };
}

function themePreset(themeId: ThemeId): ThemePreset {
  const preset = source.themePresets[themeId];
  return {
    label: preset.label,
    description: preset.description ?? '',
    appearance: preset.appearance ?? {
      style: 'askolo',
      radius: '0.875rem',
      fontFamily: ['Cairo', 'sans-serif'],
      iconLibrary: 'lucide',
      menuColor: 'default',
      menuAccent: 'subtle',
    },
    light: chartTokens(themeId, 'light'),
    dark: chartTokens(themeId, 'dark'),
  };
}

export const THEME_PRESETS: Record<ThemeId, ThemePreset> = {
  blueHorizon: themePreset('blueHorizon'),
  warmPaper: themePreset('warmPaper'),
  neutral: themePreset('neutral'),
};

export const DEFAULT_THEME_PREFERENCES: ResolvedThemePreferences = {
  accountThemeId: 'neutral',
  mode: 'dark',
  modePreference: 'dark',
  surfaceStyles: { light: 'soft', dark: 'soft' },
  spaceOverrides: {},
};

export function isThemeMode(value: unknown): value is ThemeMode {
  return value === 'light' || value === 'dark';
}

export function isThemeId(value: unknown): value is ThemeId {
  return (
    typeof value === 'string' &&
    Object.prototype.hasOwnProperty.call(THEME_PRESETS, value)
  );
}

export function isSurfaceStyleId(value: unknown): value is SurfaceStyleId {
  return value === 'soft' || value === 'tinted';
}

export function getThemeVariables(
  themeId: ThemeId,
  mode: ThemeMode,
  surfaceStyle: SurfaceStyleId,
): Record<string, string> {
  const preset = source.themePresets[themeId];
  const sharedColors = { ...source.color[mode], ...preset[mode] };
  const actionColors = { ...source.color[mode], ...preset[mode] };
  const surfaces = preset.surfaceOptions[mode][surfaceStyle];
  const color = (group: TokenColor, role: string) =>
    hexToHslChannels(readHexColor(group, role));

  const values: Record<string, string> = {
    '--background': color(surfaces, 'background'),
    '--foreground': color(sharedColors, 'foreground'),
    '--border': color(surfaces, 'border'),
    '--input': color(surfaces, 'input'),
    '--ring': color(actionColors, 'ring'),
    '--card': color(surfaces, 'card'),
    '--card-foreground': color(sharedColors, 'cardForeground'),
    '--card-border': color(surfaces, 'border'),
    '--popover': color(surfaces, 'popover'),
    '--popover-foreground': color(sharedColors, 'popoverForeground'),
    '--popover-border': color(surfaces, 'border'),
    '--primary': color(actionColors, 'primary'),
    '--primary-foreground': color(actionColors, 'primaryForeground'),
    '--secondary': color(actionColors, 'secondary'),
    '--secondary-foreground': color(actionColors, 'secondaryForeground'),
    '--muted': color(surfaces, 'muted'),
    '--muted-foreground': color(sharedColors, 'mutedForeground'),
    '--accent': color(actionColors, 'accent'),
    '--accent-foreground': color(actionColors, 'accentForeground'),
    '--destructive': color(sharedColors, 'destructive'),
    '--destructive-foreground': color(sharedColors, 'destructiveForeground'),
    '--success': color(sharedColors, 'success'),
    '--success-foreground': color(sharedColors, 'successForeground'),
    '--info': color(sharedColors, 'info'),
    '--info-foreground': color(sharedColors, 'infoForeground'),
    '--warning': color(sharedColors, 'warning'),
    '--warning-foreground': color(sharedColors, 'warningForeground'),
    '--invert': color(sharedColors, 'invert'),
    '--invert-foreground': color(sharedColors, 'invertForeground'),
    '--chart-1': color(actionColors, 'chart1'),
    '--chart-2': color(actionColors, 'chart2'),
    '--chart-3': color(actionColors, 'chart3'),
    '--chart-4': color(actionColors, 'chart4'),
    '--chart-5': color(actionColors, 'chart5'),
    '--chart-6': color(actionColors, 'chart6'),
    '--chart-7': color(actionColors, 'chart7'),
    '--chart-8': color(actionColors, 'chart8'),
    '--sidebar': color(surfaces, 'sidebar'),
    '--sidebar-foreground': color(sharedColors, 'sidebarForeground'),
    '--sidebar-border': color(surfaces, 'sidebarBorder'),
    '--sidebar-primary': color(actionColors, 'sidebarPrimary'),
    '--sidebar-primary-foreground': color(actionColors, 'sidebarPrimaryForeground'),
    '--sidebar-accent': color(actionColors, 'sidebarAccent'),
    '--sidebar-accent-foreground': color(actionColors, 'sidebarAccentForeground'),
    '--sidebar-ring': color(actionColors, 'sidebarRing'),
    '--space-personal': color(sharedColors, 'spacePersonal'),
    '--space-personal-foreground': color(sharedColors, 'spacePersonalForeground'),
    '--space-team': color(sharedColors, 'spaceTeam'),
    '--space-team-foreground': color(sharedColors, 'spaceTeamForeground'),
    '--space-family': color(sharedColors, 'spaceFamily'),
    '--space-family-foreground': color(sharedColors, 'spaceFamilyForeground'),
  };

  values['--primary-border'] = `hsl(${values['--primary']})`;
  values['--secondary-border'] = `hsl(${values['--secondary']})`;
  values['--muted-border'] = `hsl(${values['--muted']})`;
  values['--accent-border'] = `hsl(${values['--accent']})`;
  values['--destructive-border'] = `hsl(${values['--destructive']})`;
  values['--sidebar-primary-border'] = `hsl(${values['--sidebar-primary']})`;
  values['--sidebar-accent-border'] = `hsl(${values['--sidebar-accent']})`;
  values['--button-outline'] = 'hsl(var(--foreground) / 16%)';
  values['--badge-outline'] = 'hsl(var(--foreground) / 8%)';
  values['--opaque-button-border-intensity'] = mode === 'dark' ? '9' : '12';
  values['--elevate-1'] = 'hsl(var(--foreground) / 4%)';
  values['--elevate-2'] = 'hsl(var(--foreground) / 9%)';

  return values;
}

function applyTheme(value: ResolvedThemePreferences) {
  if (typeof document === 'undefined') return;
  const root = document.documentElement;
  root.classList.toggle('dark', value.mode === 'dark');
  root.style.colorScheme = value.mode;
  root.dataset.askoloTheme = value.accountThemeId;
  root.dataset.askoloMode = value.mode;
  root.dataset.askoloSurfaceStyle = value.surfaceStyles[value.mode];
  const logoPrimary = readHexColor(source.themePresets[value.accountThemeId][value.mode], 'primary');
  const logoFilter =
    value.accountThemeId === 'neutral'
      ? 'grayscale(1)'
      : (() => {
          const logoHue = Number(hexToHslChannels(logoPrimary).split(' ')[0]);
          const logoHueRotation = ((logoHue - 189 + 540) % 360) - 180;
          return `hue-rotate(${logoHueRotation}deg)`;
        })();
  root.style.setProperty('--askolo-brand-logo-filter', logoFilter);
  const iconSuffix = `${value.accountThemeId}-${value.mode}`;
  const faviconLink = root.querySelector<HTMLLinkElement>('link[rel="icon"]');
  faviconLink?.setAttribute('href', `/icons/fav-${iconSuffix}.png`);
  faviconLink?.setAttribute('type', 'image/png');
  root.querySelector<HTMLLinkElement>('link[rel="apple-touch-icon"]')?.setAttribute(
    'href',
    `/icons/touch-${iconSuffix}.png`,
  );
  const appearance = THEME_PRESETS[value.accountThemeId].appearance;
  root.dataset.askoloStyle = appearance.style;
  root.dataset.askoloIconLibrary = appearance.iconLibrary;
  root.dataset.askoloMenuColor = appearance.menuColor;
  root.dataset.askoloMenuAccent = appearance.menuAccent;
  root.style.setProperty('--app-font-sans', appearance.fontFamily.join(', '));
  root.style.setProperty('--app-font-display', appearance.fontFamily.join(', '));
  root.style.setProperty('--radius', appearance.radius);
  for (const [name, token] of Object.entries(
    getThemeVariables(value.accountThemeId, value.mode, value.surfaceStyles[value.mode]),
  )) {
    root.style.setProperty(name, token);
  }
}

interface ThemeContextValue {
  preferences: ResolvedThemePreferences;
  setAccountTheme: (themeId: ThemeId) => void;
  setMode: (mode: ThemeModePreference) => void;
  setSurfaceStyle: (surfaceStyle: SurfaceStyleId) => void;
}

const ThemeContext = createContext<ThemeContextValue | null>(null);

export function ThemeProvider({
  value,
  onChange,
  children,
}: {
  value: ResolvedThemePreferences;
  onChange: (next: ResolvedThemePreferences) => void;
  children: ReactNode;
}) {
  useEffect(() => {
    if (value.modePreference !== 'system' || typeof window === 'undefined') return;
    const media = window.matchMedia('(prefers-color-scheme: dark)');
    const syncWithSystem = () => {
      const mode: ThemeMode = media.matches ? 'dark' : 'light';
      if (value.mode !== mode) onChange({ ...value, mode });
    };
    syncWithSystem();
    media.addEventListener('change', syncWithSystem);
    return () => media.removeEventListener('change', syncWithSystem);
  }, [onChange, value]);

  const context = useMemo<ThemeContextValue>(
    () => ({
      preferences: value,
      setAccountTheme: (accountThemeId) => onChange({ ...value, accountThemeId }),
      setMode: (modePreference) => {
        const mode: ThemeMode = modePreference === 'system'
          ? (typeof window !== 'undefined' && window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light')
          : modePreference;
        onChange({ ...value, mode, modePreference });
      },
      setSurfaceStyle: (surfaceStyle) =>
        onChange({
          ...value,
          surfaceStyles: { ...value.surfaceStyles, [value.mode]: surfaceStyle },
        }),
    }),
    [onChange, value],
  );

  useLayoutEffect(() => applyTheme(value), [value]);
  return <ThemeContext.Provider value={context}>{children}</ThemeContext.Provider>;
}

export function useAskoloTheme(): ThemeContextValue {
  const context = useContext(ThemeContext);
  if (!context) throw new Error('useAskoloTheme must be used within ThemeProvider.');
  return context;
}