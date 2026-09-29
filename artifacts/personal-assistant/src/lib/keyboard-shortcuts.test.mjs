import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import {
  getCommandMenuAriaKeyshortcuts,
  getCommandMenuShortcutLabel,
  matchesCommandMenuShortcut,
} from './keyboard-shortcuts.ts';

function keyEvent(overrides = {}) {
  return {
    key: 'k',
    ctrlKey: true,
    metaKey: false,
    shiftKey: false,
    altKey: false,
    repeat: false,
    isComposing: false,
    defaultPrevented: false,
    target: null,
    ...overrides,
  };
}

test('the command binding matches the configured platform modifier exactly', () => {
  assert.equal(matchesCommandMenuShortcut(keyEvent(), 'mod+k', 'Win32'), true);
  assert.equal(
    matchesCommandMenuShortcut(
      keyEvent({ ctrlKey: false, metaKey: true }),
      'mod+k',
      'MacIntel',
    ),
    true,
  );
  assert.equal(matchesCommandMenuShortcut(keyEvent(), 'alt+shift+k', 'Win32'), false);
  assert.equal(
    matchesCommandMenuShortcut(
      keyEvent({ ctrlKey: false, altKey: true, shiftKey: true }),
      'alt+shift+k',
      'Win32',
    ),
    true,
  );
  assert.equal(
    matchesCommandMenuShortcut(
      keyEvent({ ctrlKey: true, metaKey: true }),
      'mod+k',
      'Win32',
    ),
    false,
  );
});

test('the command binding leaves other and browser-owned key interactions alone', () => {
  assert.equal(matchesCommandMenuShortcut(keyEvent({ altKey: true }), 'mod+k', 'Win32'), false);
  assert.equal(matchesCommandMenuShortcut(keyEvent({ shiftKey: true }), 'mod+k', 'Win32'), false);
  assert.equal(matchesCommandMenuShortcut(keyEvent({ repeat: true }), 'mod+k', 'Win32'), false);
  assert.equal(matchesCommandMenuShortcut(keyEvent({ isComposing: true }), 'mod+k', 'Win32'), false);
  assert.equal(matchesCommandMenuShortcut(keyEvent({ defaultPrevented: true }), 'mod+k', 'Win32'), false);
  assert.equal(matchesCommandMenuShortcut(keyEvent(), 'disabled', 'Win32'), false);
  assert.equal(
    matchesCommandMenuShortcut(
      keyEvent({ target: { closest: () => ({}) } }),
      'mod+k',
      'Win32',
    ),
    false,
  );
});

test('shortcut labels and aria-keyshortcuts describe the active binding', () => {
  assert.equal(getCommandMenuShortcutLabel('mod+k', 'Win32'), 'Ctrl+K');
  assert.equal(getCommandMenuShortcutLabel('alt+shift+k', 'MacIntel'), '⌥ Shift K');
  assert.equal(getCommandMenuAriaKeyshortcuts('alt+shift+k', 'MacIntel'), 'Alt+Shift+K');
  assert.equal(getCommandMenuAriaKeyshortcuts('disabled', 'Win32'), undefined);
});

test('reduced-motion styles disable app animation and smooth scrolling', async () => {
  const css = await readFile(new URL('../index.css', import.meta.url), 'utf8');
  const reducedMotionRule = css.match(/@media \(prefers-reduced-motion: reduce\)\s*\{([\s\S]*?)\n\}/);
  assert.ok(reducedMotionRule, 'a reduced-motion media query is present');
  assert.match(reducedMotionRule[1], /scroll-behavior:\s*auto/);
  assert.match(reducedMotionRule[1], /transition-duration:\s*0\.01ms/);
  assert.match(reducedMotionRule[1], /animation-duration:\s*0\.01ms/);
});