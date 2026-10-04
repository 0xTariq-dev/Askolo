import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import { Window } from 'happy-dom';

const themeBootstrap = await readFile(
  new URL('../../public/theme-mode-bootstrap.js', import.meta.url),
  'utf8',
);
const localeBootstrap = await readFile(
  new URL('../../public/locale-bootstrap.js', import.meta.url),
  'utf8',
);
const localeRegistry = JSON.parse(
  await readFile(new URL('./locale-registry.json', import.meta.url), 'utf8'),
);

function makeWindow(search = '') {
  return new Window({ url: `http://localhost/${search}` });
}

test('theme prepaint applies the dark Blue Horizon default before app startup', () => {
  const window = makeWindow();
  window.eval(themeBootstrap);
  const root = window.document.documentElement;

  assert.equal(root.classList.contains('dark'), true);
  assert.equal(root.dataset.askoloTheme, 'blueHorizon');
  assert.equal(root.dataset.askoloMode, 'dark');
  assert.equal(root.dataset.askoloSurfaceStyle, 'soft');
  window.close();
});

test('theme prepaint restores the saved palette, mode, and active surface', () => {
  const window = makeWindow();
  window.localStorage.setItem(
    'askolo-theme-preferences',
    JSON.stringify({
      accountThemeId: 'warmPaper',
      mode: 'light',
      surfaceStyles: { light: 'tinted', dark: 'soft' },
    }),
  );

  window.eval(themeBootstrap);
  const root = window.document.documentElement;

  assert.equal(root.classList.contains('dark'), false);
  assert.equal(root.style.colorScheme, 'light');
  assert.equal(root.dataset.askoloTheme, 'warmPaper');
  assert.equal(root.dataset.askoloMode, 'light');
  assert.equal(root.dataset.askoloSurfaceStyle, 'tinted');
  window.close();
});

test('an explicit theme query overrides saved mode without replacing saved palette', () => {
  const window = makeWindow('?theme=light');
  window.localStorage.setItem(
    'askolo-theme-preferences',
    JSON.stringify({
      accountThemeId: 'warmPaper',
      mode: 'dark',
      surfaceStyles: { light: 'tinted', dark: 'soft' },
    }),
  );

  window.eval(themeBootstrap);
  const root = window.document.documentElement;

  assert.equal(root.classList.contains('dark'), false);
  assert.equal(root.dataset.askoloTheme, 'warmPaper');
  assert.equal(root.dataset.askoloMode, 'light');
  assert.equal(root.dataset.askoloSurfaceStyle, 'tinted');
  window.close();
});

test('locale prepaint restores Arabic direction independently of theme', () => {
  const window = makeWindow();
  window.localStorage.setItem('askolo-locale', 'ar');
  window.__ASKOLO_LOCALE_BOOTSTRAP__ = {
    defaultLocale: localeRegistry.defaultLocale,
    locales: localeRegistry.locales,
  };

  window.eval(themeBootstrap);
  window.eval(localeBootstrap);

  assert.equal(window.document.documentElement.lang, 'ar');
  assert.equal(window.document.documentElement.dir, localeRegistry.locales.ar.direction);
  window.close();
});

test('locale prepaint falls back to the registry default for unsupported values', () => {
  const window = makeWindow();
  window.localStorage.setItem('askolo-locale', 'fr');
  window.__ASKOLO_LOCALE_BOOTSTRAP__ = {
    defaultLocale: localeRegistry.defaultLocale,
    locales: localeRegistry.locales,
  };

  window.eval(localeBootstrap);

  assert.equal(window.document.documentElement.lang, localeRegistry.defaultLocale);
  assert.equal(
    window.document.documentElement.dir,
    localeRegistry.locales[localeRegistry.defaultLocale].direction,
  );
  window.close();
});