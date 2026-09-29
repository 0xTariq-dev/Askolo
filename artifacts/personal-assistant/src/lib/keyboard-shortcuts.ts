export type CommandMenuShortcut = 'mod+k' | 'alt+shift+k' | 'disabled';

export const COMMAND_MENU_SHORTCUT_STORAGE_KEY =
  'askolo:keyboard-shortcuts:command-menu';

export const DEFAULT_COMMAND_MENU_SHORTCUT: CommandMenuShortcut = 'alt+shift+k';

export const COMMAND_MENU_SHORTCUT_OPTIONS: {
  value: CommandMenuShortcut;
  label: string;
}[] = [
  { value: 'alt+shift+k', label: 'Alt/Option+Shift+K (recommended)' },
  { value: 'mod+k', label: 'Ctrl/Cmd+K (may conflict with browser search)' },
  { value: 'disabled', label: 'Disabled' },
];

export function isCommandMenuShortcut(
  value: unknown,
): value is CommandMenuShortcut {
  return (
    value === 'mod+k' ||
    value === 'alt+shift+k' ||
    value === 'disabled'
  );
}

export function loadCommandMenuShortcut(): CommandMenuShortcut {
  if (typeof window === 'undefined') return DEFAULT_COMMAND_MENU_SHORTCUT;

  try {
    const saved = window.localStorage.getItem(COMMAND_MENU_SHORTCUT_STORAGE_KEY);
    return isCommandMenuShortcut(saved)
      ? saved
      : DEFAULT_COMMAND_MENU_SHORTCUT;
  } catch {
    return DEFAULT_COMMAND_MENU_SHORTCUT;
  }
}

export function saveCommandMenuShortcut(
  shortcut: CommandMenuShortcut,
): boolean {
  if (typeof window === 'undefined') return false;

  try {
    window.localStorage.setItem(COMMAND_MENU_SHORTCUT_STORAGE_KEY, shortcut);
    return true;
  } catch {
    return false;
  }
}

function isApplePlatform(platform: string): boolean {
  return /Mac|iPhone|iPad|iPod/i.test(platform);
}

function getPlatformName(platform?: string): string {
  if (platform) return platform;
  if (typeof navigator === 'undefined') return '';

  return navigator.platform || '';
}

export function getCommandMenuShortcutLabel(
  shortcut: CommandMenuShortcut,
  platform?: string,
): string {
  if (shortcut === 'disabled') return 'Disabled';

  const modifier = isApplePlatform(getPlatformName(platform)) ? '⌘' : 'Ctrl';
  if (shortcut === 'alt+shift+k') {
    return modifier === '⌘' ? '⌥ Shift K' : 'Alt+Shift+K';
  }
  return modifier === '⌘' ? '⌘ K' : 'Ctrl+K';
}

export function getCommandMenuAriaKeyshortcuts(
  shortcut: CommandMenuShortcut,
  platform?: string,
): string | undefined {
  if (shortcut === 'disabled') return undefined;

  if (shortcut === 'alt+shift+k') return 'Alt+Shift+K';
  return isApplePlatform(getPlatformName(platform))
    ? 'Meta+K'
    : 'Control+K';
}

type ShortcutKeyboardEvent = Pick<
  KeyboardEvent,
  | 'key'
  | 'ctrlKey'
  | 'metaKey'
  | 'shiftKey'
  | 'altKey'
  | 'repeat'
  | 'isComposing'
  | 'defaultPrevented'
  | 'target'
>;

function targetsEditableControl(target: EventTarget | null): boolean {
  if (!target || typeof (target as Element).closest !== 'function') return false;

  return Boolean(
    (target as Element).closest(
      'input:not([type="button"]):not([type="submit"]):not([type="reset"]), textarea, select, [contenteditable]:not([contenteditable="false"]), [role="textbox"]',
    ),
  );
}

export function matchesCommandMenuShortcut(
  event: ShortcutKeyboardEvent,
  shortcut: CommandMenuShortcut,
  platform?: string,
): boolean {
  if (
    shortcut === 'disabled' ||
    event.defaultPrevented ||
    event.isComposing ||
    event.repeat ||
    event.key.toLowerCase() !== 'k' ||
    targetsEditableControl(event.target)
  ) {
    return false;
  }

  const usesAltBinding = shortcut === 'alt+shift+k';
  const hasExpectedModifier = usesAltBinding
    ? event.altKey && !event.ctrlKey && !event.metaKey
    : isApplePlatform(getPlatformName(platform))
      ? event.metaKey && !event.ctrlKey && !event.altKey
      : event.ctrlKey && !event.metaKey && !event.altKey;
  const requiresShift = usesAltBinding;

  return hasExpectedModifier && event.shiftKey === requiresShift;
}