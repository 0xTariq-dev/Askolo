import type { KeyboardEvent } from 'react';
import { IconDeviceDesktop, IconMoon, IconSun } from '@tabler/icons-react';
import { Button } from '@/components/ui/button';
import {
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
} from '@/components/ui/dropdown-menu';
import { useLocale } from '@/contexts/locale-context';
import { useAskoloTheme, type ThemeModePreference } from '@/lib/theme';

const modes = [
  { id: 'light', label: { en: 'Light', ar: 'فاتح' }, Icon: IconSun },
  { id: 'dark', label: { en: 'Dark', ar: 'داكن' }, Icon: IconMoon },
  { id: 'system', label: { en: 'System', ar: 'النظام' }, Icon: IconDeviceDesktop },
] as const satisfies readonly {
  id: ThemeModePreference;
  label: { en: string; ar: string };
  Icon: typeof IconSun;
}[];

type ThemeModeToggleProps = {
  placement?: 'menu' | 'settings';
};

export function ThemeModeToggle({ placement = 'settings' }: ThemeModeToggleProps) {
  const { preferences, setMode } = useAskoloTheme();
  const { locale } = useLocale();
  const compact = placement === 'menu';
  const modeLabel = (id: ThemeModePreference) =>
    modes.find((mode) => mode.id === id)?.label[locale === 'ar' ? 'ar' : 'en'] ?? id;
  const groupClass = `app-border-subtle flex items-center rounded-full bg-muted/70 ${compact ? 'gap-0.5 p-0.5' : 'gap-1.5 p-1.5'}`;
  const optionClass = `app-border-subtle flex items-center justify-center rounded-full p-0 text-muted-foreground transition-colors focus-visible:border-ring focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 ${
    compact ? 'size-6' : 'size-10'
  }`;

  const handleSettingsKeyDown = (
    event: KeyboardEvent<HTMLButtonElement>,
    index: number,
  ) => {
    const key = event.key;
    if (!['ArrowRight', 'ArrowLeft', 'ArrowDown', 'ArrowUp', 'Home', 'End'].includes(key)) return;
    event.preventDefault();
    const radios = Array.from(
      event.currentTarget.parentElement?.querySelectorAll<HTMLButtonElement>('[role="radio"]') ?? [],
    );
    if (!radios.length) return;
    const rtl = getComputedStyle(event.currentTarget).direction === 'rtl';
    const nextIndex = key === 'Home'
      ? 0
      : key === 'End'
        ? radios.length - 1
        : key === 'ArrowDown' || key === 'ArrowRight' && !rtl || key === 'ArrowLeft' && rtl
          ? (index + 1) % radios.length
          : (index - 1 + radios.length) % radios.length;
    const nextMode = modes[nextIndex];
    radios[nextIndex]?.focus();
    if (nextMode) setMode(nextMode.id);
  };

  return (
    <div className={groupClass}>
      {compact ? (
        <DropdownMenuRadioGroup
          aria-label={locale === 'ar' ? 'اختيار المظهر' : 'Appearance mode'}
          value={preferences.modePreference}
          onValueChange={(value) => {
            if (value === 'light' || value === 'dark' || value === 'system') setMode(value);
          }}
          className="flex items-center gap-0.5"
        >
          {modes.map(({ id, Icon }) => (
            <DropdownMenuRadioItem
              key={id}
              value={id}
              className={`${optionClass} data-[state=checked]:border-primary data-[state=checked]:bg-primary data-[state=checked]:text-primary-foreground data-[state=checked]:focus:bg-primary data-[state=checked]:focus:text-primary-foreground [&>[data-slot=dropdown-menu-radio-item-indicator]]:hidden`}
              data-testid={`menu-theme-${id}`}
            >
              <Icon className="size-3" aria-hidden="true" />
              <span className="sr-only">{modeLabel(id)}</span>
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      ) : (
        <div
          role="radiogroup"
          aria-label={locale === 'ar' ? 'اختيار المظهر' : 'Appearance mode'}
          className="flex items-center gap-1.5"
        >
          {modes.map(({ id, Icon }, index) => {
            const selected = preferences.modePreference === id;
            return (
              <Button
                key={id}
                type="button"
                variant="ghost"
                role="radio"
                aria-label={modeLabel(id)}
                aria-checked={selected}
                tabIndex={selected ? 0 : -1}
                onClick={() => setMode(id)}
                onKeyDown={(event) => handleSettingsKeyDown(event, index)}
                className={`${optionClass} aria-checked:border-primary aria-checked:bg-primary aria-checked:text-primary-foreground aria-checked:hover:bg-primary hover:bg-accent`}
                data-testid={`settings-theme-${id}`}
              >
                <Icon className="size-4" aria-hidden="true" />
              </Button>
            );
          })}
        </div>
      )}
    </div>
  );
}
