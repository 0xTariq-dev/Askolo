import { Button } from '@/components/ui/button';
import {
  isThemeId,
  getThemeVariables,
  THEME_PRESETS,
  useAskoloTheme,
  type ThemeId,
  type ThemeMode,
} from '@/lib/theme';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';

const THEME_OPTIONS = ['blueHorizon', 'warmPaper', 'neutral'] as const satisfies readonly ThemeId[];
const MODES: readonly { id: ThemeMode; label: string }[] = [
  { id: 'light', label: 'Light' },
  { id: 'dark', label: 'Dark' },
];

function PresetSwatches({
  themeId,
  mode,
  surfaceStyle,
}: {
  themeId: ThemeId;
  mode: ThemeMode;
  surfaceStyle: 'soft' | 'tinted';
}) {
  const variables = getThemeVariables(themeId, mode, surfaceStyle);
  return (
    <span aria-hidden="true" className="flex shrink-0 -space-x-2">
      {['--primary', '--accent', '--background', '--foreground'].map((token) => (
        <span
          key={token}
          className="size-4 rounded-full border-2 border-popover shadow-sm"
          style={{ backgroundColor: `hsl(${variables[token]})` }}
        />
      ))}
    </span>
  );
}

export function ThemePresetSelector() {
  const { preferences, setAccountTheme, setMode } = useAskoloTheme();
  const mode = preferences.mode;
  const surfaceStyle = preferences.surfaceStyles[mode];

  return (
    <section aria-labelledby="appearance-settings-title" className="space-y-5">
      <header className="space-y-1">
        <h2 id="appearance-settings-title" className="text-base font-semibold">Appearance</h2>
        <p className="text-sm text-muted-foreground">
          Presets control colors, typography, component style, radius, and icon library.
        </p>
      </header>
        <fieldset className="space-y-2">
          <legend className="text-sm font-medium">Mode</legend>
          <div role="group" aria-label="Appearance mode" className="flex gap-2">
            {MODES.map((option) => (
              <Button
                key={option.id}
                type="button"
                size="sm"
                variant={mode === option.id ? 'default' : 'outline'}
                aria-pressed={mode === option.id}
                onClick={() => setMode(option.id)}
                className="min-h-10 motion-reduce:transition-none"
              >
                {option.label}
              </Button>
            ))}
          </div>
        </fieldset>

        <fieldset className="space-y-2">
          <legend className="text-sm font-medium">Theme preset</legend>
          <Select
            value={preferences.accountThemeId}
            onValueChange={(value) => {
              if (isThemeId(value)) setAccountTheme(value);
            }}
          >
            <SelectTrigger id="theme-preset" aria-label="Theme preset" className="w-full max-w-md rounded-[var(--radius)]">
              <SelectValue />
            </SelectTrigger>
            <SelectContent
              position="popper"
              className="w-(--radix-select-trigger-width) max-w-[calc(100vw-2rem)] rounded-[var(--radius)]"
            >
              {THEME_OPTIONS.map((themeId) => (
                <SelectItem
                  key={themeId}
                  value={themeId}
                  className="rounded-[calc(var(--radius)*0.7)] py-2.5"
                >
                  <span className="flex min-w-0 flex-1 items-center justify-between gap-4">
                    <span className="truncate font-medium">{THEME_PRESETS[themeId].label}</span>
                    <PresetSwatches themeId={themeId} mode={mode} surfaceStyle={surfaceStyle} />
                  </span>
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </fieldset>
    </section>
  );
}