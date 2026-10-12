import { useEffect, useId, useRef, useState } from 'react';
import { useLocation } from 'wouter';
import { Search, type LucideIcon } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  Command,
  CommandInput,
  CommandList,
  CommandEmpty,
  CommandGroup,
  CommandItem,
  CommandShortcut,
} from '@/components/ui/command';
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';
import { useKeyboardShortcutPreferences } from '@/contexts/keyboard-shortcut-context';
import {
  getCommandMenuAriaKeyshortcuts,
  getCommandMenuShortcutLabel,
  matchesCommandMenuShortcut,
} from '@/lib/keyboard-shortcuts';

export type CommandDestination = {
  href: string;
  label: string;
  icon: LucideIcon;
};

export function CommandMenu({
  commands,
  collapsed = false,
}: {
  commands: CommandDestination[];
  collapsed?: boolean;
}) {
  const [, setLocation] = useLocation();
  const { commandMenuShortcut } = useKeyboardShortcutPreferences();
  const [open, setOpen] = useState(false);
  const instructionsId = useId();
  const triggerRef = useRef<HTMLButtonElement>(null);
  const restoreFocusRef = useRef<HTMLElement | null>(null);

  const openFromCurrentFocus = () => {
    restoreFocusRef.current =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : triggerRef.current;
    setOpen(true);
  };

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (
        open ||
        document.querySelector('[role="dialog"][data-state="open"]') ||
        !matchesCommandMenuShortcut(event, commandMenuShortcut)
      ) {
        return;
      }

      event.preventDefault();
      openFromCurrentFocus();
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [commandMenuShortcut, open]);

  const handleCloseAutoFocus = (event: Event) => {
    event.preventDefault();
    const target = restoreFocusRef.current;
    if (target?.isConnected) {
      target.focus();
    } else {
      triggerRef.current?.focus();
    }
  };

  const shortcutLabel = getCommandMenuShortcutLabel(commandMenuShortcut);
  const ariaKeyshortcuts =
    getCommandMenuAriaKeyshortcuts(commandMenuShortcut);

  return (
    <>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <Button
            ref={triggerRef}
            type="button"
            variant="outline"
            className={`shrink-0 gap-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 ${
              collapsed ? 'size-8 justify-center p-0' : 'w-full justify-start'
            }`}
            title={ariaKeyshortcuts
              ? `Open search (${shortcutLabel})`
              : 'Open search'}
            aria-label={
              ariaKeyshortcuts
                ? `Open search (${shortcutLabel})`
                : 'Open search'
            }
            aria-keyshortcuts={ariaKeyshortcuts}
            onClick={openFromCurrentFocus}
            data-testid="open-search"
          >
            <Search className="h-4 w-4" aria-hidden="true" />
            {!collapsed && <span>Search</span>}
            {!collapsed && commandMenuShortcut !== 'disabled' && (
              <span
                aria-hidden="true"
                className="hidden rounded border border-border px-1.5 py-0.5 text-xs text-muted-foreground sm:inline"
              >
                {shortcutLabel}
              </span>
            )}
          </Button>
        </PopoverTrigger>
        <PopoverContent
          side="bottom"
          align="start"
          sideOffset={6}
          aria-label="Search pages"
          aria-describedby={instructionsId}
          onCloseAutoFocus={handleCloseAutoFocus}
          className="relative max-h-[min(32rem,calc(100dvh-2rem))] w-[min(24rem,calc(100vw-1rem))] max-w-[calc(100vw-1rem)] gap-0 overflow-hidden rounded-[var(--radius)] bg-popover/70 p-1 text-popover-foreground shadow-2xl ring-1 ring-foreground/5 before:pointer-events-none before:absolute before:inset-0 before:-z-1 before:rounded-[inherit] before:backdrop-blur-2xl before:backdrop-saturate-150"
        >
          <p id={instructionsId} className="sr-only">
            Use the arrow keys to move through results, Enter to open a page, and Escape to close.
          </p>
          <Command className="max-h-[min(30rem,calc(100dvh-4rem))] rounded-[calc(var(--radius)*0.7)] bg-transparent p-0">
            <CommandInput aria-label="Search pages" placeholder="Search pages..." />
            <CommandList>
              <CommandEmpty>No matching pages found.</CommandEmpty>
              <CommandGroup heading="Navigate">
                {commands.map(({ href, label, icon: Icon }) => (
                  <CommandItem
                    key={href}
                    value={`${label} ${href}`}
                    onSelect={() => {
                      setOpen(false);
                      setLocation(href);
                    }}
                  >
                    <Icon aria-hidden="true" />
                    <span>{label}</span>
                    <CommandShortcut>Enter</CommandShortcut>
                  </CommandItem>
                ))}
              </CommandGroup>
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>
    </>
  );
}