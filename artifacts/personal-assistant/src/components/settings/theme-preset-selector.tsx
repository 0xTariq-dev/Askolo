import { Check } from 'lucide-react';
import { Button } from '@workspace/askolo-design-system/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@workspace/askolo-design-system/components/ui/card';
import {
  getThemeVariables,
  THEME_PRESETS,
  useAskoloTheme,
  type ThemeId,
  type ThemeMode,
} from '@workspace/askolo-design-system/theme';

const THEME_OPTIONS = ['blueHorizon', 'warmPaper'] as const satisfies readonly ThemeId[];
const MODES: readonly { id: ThemeMode; label: string }[] = [
  { id: 'light', label: 'Light' },
  { id: 'dark', label: 'Dark' },
];

export function ThemePresetSelector() {
  const { preferences, setAccountTheme, setMode } = useAskoloTheme();
  const mode = preferences.mode;
  const surfaceStyle = preferences.surfaceStyles[mode];

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Appearance</CardTitle>
        <CardDescription>
          Choose a light or dark appearance and one of two color presets. Presets use shared
          design tokens for backgrounds, accents, charts, and focus colors.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-5">
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
          <legend className="text-sm font-medium">Color preset</legend>
          <div
            role="group"
            aria-label={`${mode === 'light' ? 'Light' : 'Dark'} color presets`}
            className="grid grid-cols-1 gap-3 sm:grid-cols-2"
          >
            {THEME_OPTIONS.map((themeId) => {
              const preset = THEME_PRESETS[themeId];
              const variables = getThemeVariables(themeId, mode, surfaceStyle);
              const selected = preferences.accountThemeId === themeId;
              const previewStyle = {
                backgroundColor: `hsl(${variables['--card']})`,
                borderColor: `hsl(${variables['--border']})`,
                color: 'hsl(var(--foreground))',
              };
              const accentStyle = {
                backgroundColor: `hsl(${variables['--primary']})`,
                color: `hsl(${variables['--primary-foreground']})`,
              };

              return (
                <Button
                  key={themeId}
                  type="button"
                  variant="outline"
                  aria-label={`Use ${preset.label} in ${mode} mode`}
                  aria-pressed={selected}
                  onClick={() => setAccountTheme(themeId)}
                  className={`h-auto min-h-0 w-full justify-start whitespace-normal rounded-xl p-1 text-left motion-reduce:transition-none ${
                    selected ? 'border-primary ring-2 ring-primary/30' : ''
                  }`}
                >
                  <span className="block w-full rounded-lg border p-3" style={previewStyle}>
                    <span className="flex items-center justify-between gap-2">
                      <span className="font-medium">{preset.label}</span>
                      {selected && <Check className="h-4 w-4 shrink-0" aria-hidden="true" />}
                    </span>
                    <span className="mt-1 block text-xs text-muted-foreground">
                      {mode === 'light' ? 'Light' : 'Dark'} · {surfaceStyle === 'soft' ? 'Soft' : 'Tinted'} surfaces
                    </span>
                    <span className="mt-3 flex items-center justify-between gap-3">
                      <span className="text-xs">Accent preview</span>
                      <span className="rounded-md px-2 py-1 text-xs font-medium" style={accentStyle}>
                        Add a task
                      </span>
                    </span>
                  </span>
                </Button>
              );
            })}
          </div>
        </fieldset>
      </CardContent>
    </Card>
  );
}