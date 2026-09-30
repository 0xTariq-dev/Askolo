import { createContext, useContext, useMemo, type ReactNode } from 'react';

export type ThemeMode = 'light' | 'dark';
export type ThemeId = 'blueHorizon' | 'warmPaper';
export type SurfaceStyleId = 'soft' | 'tinted';

export interface ResolvedThemePreferences {
  accountThemeId: ThemeId;
  mode: ThemeMode;
  surfaceStyles: Record<ThemeMode, SurfaceStyleId>;
  spaceOverrides: Record<string, ThemeId>;
}

interface ThemeChartTokens {
  chart1: string;
  chart2: string;
  chart3: string;
  chart4: string;
  chart5: string;
}

interface ThemePreset {
  label: string;
  light: ThemeChartTokens;
  dark: ThemeChartTokens;
}

export const THEME_PRESETS: Record<ThemeId, ThemePreset> = {
  blueHorizon: {
    label: 'Blue Horizon',
    light: {
      chart1: '201 88% 43%',
      chart2: '38 84% 48%',
      chart3: '155 55% 39%',
      chart4: '270 58% 56%',
      chart5: '0 72% 52%',
    },
    dark: {
      chart1: '198 92% 58%',
      chart2: '38 92% 58%',
      chart3: '152 56% 52%',
      chart4: '280 70% 68%',
      chart5: '0 84% 66%',
    },
  },
  warmPaper: {
    label: 'Warm Paper',
    light: {
      chart1: '25 79% 46%',
      chart2: '197 68% 42%',
      chart3: '143 42% 39%',
      chart4: '273 48% 54%',
      chart5: '0 67% 52%',
    },
    dark: {
      chart1: '32 90% 62%',
      chart2: '195 77% 61%',
      chart3: '145 49% 57%',
      chart4: '278 62% 70%',
      chart5: '0 74% 66%',
    },
  },
};

export const DEFAULT_THEME_PREFERENCES: ResolvedThemePreferences = {
  accountThemeId: 'blueHorizon',
  mode: 'dark',
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

interface ThemeColorTokens {
  background: string;
  foreground: string;
  border: string;
  input: string;
  ring: string;
  cardSoft: string;
  cardTinted: string;
  cardForeground: string;
  popoverForeground: string;
  primary: string;
  primaryForeground: string;
  secondary: string;
  secondaryForeground: string;
  muted: string;
  mutedForeground: string;
  accent: string;
  accentForeground: string;
  destructive: string;
  destructiveForeground: string;
  sidebar: string;
  sidebarForeground: string;
  sidebarBorder: string;
  sidebarAccent: string;
  sidebarAccentForeground: string;
  success: string;
  successForeground: string;
  warning: string;
  warningForeground: string;
}

const PALETTE: Record<ThemeId, Record<ThemeMode, ThemeColorTokens>> = {
  blueHorizon: {
    light: {
      background: '210 40% 98%',
      foreground: '222 47% 11%',
      border: '214 32% 88%',
      input: '214 32% 88%',
      ring: '201 88% 43%',
      cardSoft: '0 0% 100%',
      cardTinted: '207 35% 96%',
      cardForeground: '222 47% 11%',
      popoverForeground: '222 47% 11%',
      primary: '201 88% 43%',
      primaryForeground: '0 0% 100%',
      secondary: '210 30% 92%',
      secondaryForeground: '222 47% 18%',
      muted: '210 30% 94%',
      mutedForeground: '215 16% 42%',
      accent: '199 70% 91%',
      accentForeground: '204 75% 25%',
      destructive: '0 72% 52%',
      destructiveForeground: '0 0% 100%',
      sidebar: '210 40% 96%',
      sidebarForeground: '222 47% 11%',
      sidebarBorder: '214 32% 88%',
      sidebarAccent: '199 70% 91%',
      sidebarAccentForeground: '204 75% 25%',
      success: '142 62% 35%',
      successForeground: '0 0% 100%',
      warning: '38 88% 43%',
      warningForeground: '0 0% 100%',
    },
    dark: {
      background: '222 43% 6%',
      foreground: '210 40% 98%',
      border: '215 28% 17%',
      input: '215 28% 17%',
      ring: '198 92% 58%',
      cardSoft: '221 38% 10%',
      cardTinted: '223 39% 13%',
      cardForeground: '210 40% 98%',
      popoverForeground: '210 40% 98%',
      primary: '38 92% 50%',
      primaryForeground: '222 43% 6%',
      secondary: '215 28% 17%',
      secondaryForeground: '210 40% 98%',
      muted: '221 38% 10%',
      mutedForeground: '215 16% 65%',
      accent: '215 28% 17%',
      accentForeground: '210 40% 98%',
      destructive: '0 84% 60%',
      destructiveForeground: '210 40% 98%',
      sidebar: '222 43% 6%',
      sidebarForeground: '210 40% 98%',
      sidebarBorder: '215 28% 17%',
      sidebarAccent: '215 28% 17%',
      sidebarAccentForeground: '210 40% 98%',
      success: '142 65% 48%',
      successForeground: '222 43% 6%',
      warning: '38 92% 58%',
      warningForeground: '222 43% 6%',
    },
  },
  warmPaper: {
    light: {
      background: '38 43% 96%',
      foreground: '25 25% 15%',
      border: '32 25% 83%',
      input: '32 25% 83%',
      ring: '25 79% 46%',
      cardSoft: '42 100% 99%',
      cardTinted: '36 45% 92%',
      cardForeground: '25 25% 15%',
      popoverForeground: '25 25% 15%',
      primary: '25 79% 46%',
      primaryForeground: '0 0% 100%',
      secondary: '34 31% 88%',
      secondaryForeground: '25 25% 18%',
      muted: '35 31% 91%',
      mutedForeground: '26 14% 40%',
      accent: '32 55% 88%',
      accentForeground: '25 49% 25%',
      destructive: '0 68% 49%',
      destructiveForeground: '0 0% 100%',
      sidebar: '38 37% 93%',
      sidebarForeground: '25 25% 15%',
      sidebarBorder: '32 25% 83%',
      sidebarAccent: '32 55% 88%',
      sidebarAccentForeground: '25 49% 25%',
      success: '142 53% 32%',
      successForeground: '0 0% 100%',
      warning: '29 81% 41%',
      warningForeground: '0 0% 100%',
    },
    dark: {
      background: '24 23% 8%',
      foreground: '40 28% 94%',
      border: '28 20% 22%',
      input: '28 20% 22%',
      ring: '32 90% 62%',
      cardSoft: '27 25% 13%',
      cardTinted: '28 29% 17%',
      cardForeground: '40 28% 94%',
      popoverForeground: '40 28% 94%',
      primary: '32 90% 62%',
      primaryForeground: '24 23% 8%',
      secondary: '28 20% 22%',
      secondaryForeground: '40 28% 94%',
      muted: '27 25% 13%',
      mutedForeground: '32 13% 68%',
      accent: '28 20% 22%',
      accentForeground: '40 28% 94%',
      destructive: '0 74% 62%',
      destructiveForeground: '24 23% 8%',
      sidebar: '24 23% 8%',
      sidebarForeground: '40 28% 94%',
      sidebarBorder: '28 20% 22%',
      sidebarAccent: '28 20% 22%',
      sidebarAccentForeground: '40 28% 94%',
      success: '142 56% 52%',
      successForeground: '24 23% 8%',
      warning: '32 90% 62%',
      warningForeground: '24 23% 8%',
    },
  },
};

export function getThemeVariables(
  themeId: ThemeId,
  mode: ThemeMode,
  surfaceStyle: SurfaceStyleId,
): Record<string, string> {
  const colors = PALETTE[themeId][mode];
  const card = surfaceStyle === 'soft' ? colors.cardSoft : colors.cardTinted;
  const charts = THEME_PRESETS[themeId][mode];
  const values: Record<string, string> = {
    '--background': colors.background,
    '--foreground': colors.foreground,
    '--border': colors.border,
    '--input': colors.input,
    '--ring': colors.ring,
    '--card': card,
    '--card-foreground': colors.cardForeground,
    '--card-border': colors.border,
    '--popover': card,
    '--popover-foreground': colors.popoverForeground,
    '--popover-border': colors.border,
    '--primary': colors.primary,
    '--primary-foreground': colors.primaryForeground,
    '--secondary': colors.secondary,
    '--secondary-foreground': colors.secondaryForeground,
    '--muted': colors.muted,
    '--muted-foreground': colors.mutedForeground,
    '--accent': colors.accent,
    '--accent-foreground': colors.accentForeground,
    '--destructive': colors.destructive,
    '--destructive-foreground': colors.destructiveForeground,
    '--sidebar': colors.sidebar,
    '--sidebar-foreground': colors.sidebarForeground,
    '--sidebar-border': colors.sidebarBorder,
    '--sidebar-primary': colors.primary,
    '--sidebar-primary-foreground': colors.primaryForeground,
    '--sidebar-accent': colors.sidebarAccent,
    '--sidebar-accent-foreground': colors.sidebarAccentForeground,
    '--sidebar-ring': colors.ring,
    '--success': colors.success,
    '--success-foreground': colors.successForeground,
    '--warning': colors.warning,
    '--warning-foreground': colors.warningForeground,
    '--chart-1': charts.chart1,
    '--chart-2': charts.chart2,
    '--chart-3': charts.chart3,
    '--chart-4': charts.chart4,
    '--chart-5': charts.chart5,
    '--primary-border': `hsl(${colors.primary})`,
    '--secondary-border': `hsl(${colors.secondary})`,
    '--muted-border': `hsl(${colors.muted})`,
    '--accent-border': `hsl(${colors.accent})`,
    '--destructive-border': `hsl(${colors.destructive})`,
    '--sidebar-primary-border': `hsl(${colors.primary})`,
    '--sidebar-accent-border': `hsl(${colors.sidebarAccent})`,
    '--button-outline': mode === 'dark' ? 'rgba(255,255,255,.16)' : 'rgba(20,35,55,.14)',
    '--badge-outline': mode === 'dark' ? 'rgba(255,255,255,.08)' : 'rgba(20,35,55,.08)',
    '--opaque-button-border-intensity': mode === 'dark' ? '9' : '12',
    '--elevate-1': mode === 'dark' ? 'rgba(255,255,255,.04)' : 'rgba(20,35,55,.04)',
    '--elevate-2': mode === 'dark' ? 'rgba(255,255,255,.09)' : 'rgba(20,35,55,.08)',
  };
  return values;
}

function applyTheme(value: ResolvedThemePreferences) {
  if (typeof document === 'undefined') return;
  const root = document.documentElement;
  const surfaceStyle = value.surfaceStyles[value.mode];
  root.classList.toggle('dark', value.mode === 'dark');
  root.style.colorScheme = value.mode;
  root.dataset.askoloTheme = value.accountThemeId;
  root.dataset.askoloSurfaceStyle = surfaceStyle;
  for (const [name, token] of Object.entries(
    getThemeVariables(value.accountThemeId, value.mode, surfaceStyle),
  )) {
    root.style.setProperty(name, token);
  }
}

interface ThemeContextValue {
  preferences: ResolvedThemePreferences;
  setAccountTheme: (themeId: ThemeId) => void;
  setMode: (mode: ThemeMode) => void;
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
  const context = useMemo<ThemeContextValue>(
    () => ({
      preferences: value,
      setAccountTheme: (accountThemeId) => onChange({ ...value, accountThemeId }),
      setMode: (mode) => onChange({ ...value, mode }),
      setSurfaceStyle: (surfaceStyle) =>
        onChange({
          ...value,
          surfaceStyles: { ...value.surfaceStyles, [value.mode]: surfaceStyle },
        }),
    }),
    [onChange, value],
  );

  applyTheme(value);
  return <ThemeContext.Provider value={context}>{children}</ThemeContext.Provider>;
}

export function useAskoloTheme(): ThemeContextValue {
  const context = useContext(ThemeContext);
  if (!context) throw new Error('useAskoloTheme must be used within ThemeProvider.');
  return context;
}