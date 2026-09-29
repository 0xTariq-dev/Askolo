import { useEffect, useRef, useState } from 'react';
import { useLocation } from 'wouter';
import { Search, type LucideIcon } from 'lucide-react';
import { Button } from '@workspace/askolo-design-system/components/ui/button';
import {
  CommandDialog,
  CommandInput,
  CommandList,
  CommandEmpty,
  CommandGroup,
  CommandItem,
  CommandShortcut,
} from '@workspace/askolo-design-system/components/ui/command';
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
}: {
  commands: CommandDestination[];
}) {
  const [, setLocation] = useLocation();
  const { commandMenuShortcut } = useKeyboardShortcutPreferences();
  const [open, setOpen] = useState(false);
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
      <Button
        ref={triggerRef}
        type="button"
        variant="outline"
        className="shrink-0 gap-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
        aria-label={
          ariaKeyshortcuts
            ? `Open command menu (${shortcutLabel})`
            : 'Open command menu'
        }
        aria-keyshortcuts={ariaKeyshortcuts}
        onClick={openFromCurrentFocus}
        data-testid="open-command-menu"
      >
        <Search className="h-4 w-4" aria-hidden="true" />
        <span>Commands</span>
        {commandMenuShortcut !== 'disabled' && (
          <span
            aria-hidden="true"
            className="hidden rounded border border-border px-1.5 py-0.5 text-xs text-muted-foreground sm:inline"
          >
            {shortcutLabel}
          </span>
        )}
      </Button>

      <CommandDialog
        open={open}
        onOpenChange={setOpen}
        title="Askolo command menu"
        description="Search available pages. Use the arrow keys to move, Enter to open a page, and Escape to close."
        onCloseAutoFocus={handleCloseAutoFocus}
      >
        <CommandInput placeholder="Search pages..." />
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
      </CommandDialog>
    </>
  );
}