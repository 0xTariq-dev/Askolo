import {
  createContext,
  useCallback,
  useContext,
  useState,
  type ReactNode,
} from 'react';
import {
  loadCommandMenuShortcut,
  saveCommandMenuShortcut,
  type CommandMenuShortcut,
} from '@/lib/keyboard-shortcuts';

type KeyboardShortcutPreferences = {
  commandMenuShortcut: CommandMenuShortcut;
  setCommandMenuShortcut: (shortcut: CommandMenuShortcut) => void;
};

const KeyboardShortcutContext =
  createContext<KeyboardShortcutPreferences | null>(null);

export function KeyboardShortcutProvider({
  children,
}: {
  children: ReactNode;
}) {
  const [commandMenuShortcut, setCommandMenuShortcutState] =
    useState<CommandMenuShortcut>(loadCommandMenuShortcut);

  const setCommandMenuShortcut = useCallback(
    (shortcut: CommandMenuShortcut) => {
      setCommandMenuShortcutState(shortcut);
      if (!saveCommandMenuShortcut(shortcut)) {
        console.warn('Askolo could not save the keyboard shortcut preference.');
      }
    },
    [],
  );

  return (
    <KeyboardShortcutContext.Provider
      value={{ commandMenuShortcut, setCommandMenuShortcut }}
    >
      {children}
    </KeyboardShortcutContext.Provider>
  );
}

export function useKeyboardShortcutPreferences() {
  const context = useContext(KeyboardShortcutContext);
  if (!context) {
    throw new Error(
      'useKeyboardShortcutPreferences must be used within KeyboardShortcutProvider.',
    );
  }
  return context;
}