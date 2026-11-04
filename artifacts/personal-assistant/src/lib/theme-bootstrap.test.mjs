import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import { Window } from 'happy-dom';

const themeBootstrap = await readFile(
  new URL('../../public/scripts/theme-mode-bootstrap.js', import.meta.url),
  'utf8',
);
const localeBootstrap = await readFile(
  new URL('../../public/scripts/locale-bootstrap.js', import.meta.url),
  'utf8',
);
const localeRegistry = JSON.parse(
  await readFile(new URL('./locale-registry.json', import.meta.url), 'utf8'),
);

function makeWindow(search = '') {
  return new Window({ url: `http://localhost/${search}` });
}

test('theme prepaint applies the dark Neutral default before app startup', () => {
  const window = makeWindow();
  window.eval(themeBootstrap);
  const root = window.document.documentElement;

  assert.equal(root.classList.contains('dark'), true);
  assert.equal(root.dataset.askoloTheme, 'neutral');
  assert.equal(root.dataset.askoloStyle, 'radix-maia');
  assert.equal(root.dataset.askoloIconLibrary, 'tabler');
  assert.equal(root.dataset.askoloMode, 'dark');
  assert.equal(root.dataset.askoloSurfaceStyle, 'soft');
  window.close();
});

test('theme prepaint migrates the saved Neutral Maia preset to Neutral', () => {
  const window = makeWindow();
  window.localStorage.setItem(
    'askolo-theme-preferences',
    JSON.stringify({ accountThemeId: 'neutralMaia', mode: 'dark' }),
  );

  window.eval(themeBootstrap);

  assert.equal(window.document.documentElement.dataset.askoloTheme, 'neutral');
  assert.equal(window.document.documentElement.dataset.askoloStyle, 'radix-maia');
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
  assert.equal(root.dataset.askoloIconLibrary, 'tabler');
  assert.equal(root.dataset.askoloMode, 'light');
  assert.equal(root.dataset.askoloSurfaceStyle, 'tinted');
  window.close();
});

test('theme prepaint resolves a saved System preference before styles render', () => {
  const window = makeWindow();
  Object.defineProperty(window, 'matchMedia', {
    value: () => ({ matches: true }),
  });
  window.localStorage.setItem(
    'askolo-theme-preferences',
    JSON.stringify({
      accountThemeId: 'warmPaper',
      mode: 'light',
      modePreference: 'system',
      surfaceStyles: { light: 'soft', dark: 'tinted' },
    }),
  );

  window.eval(themeBootstrap);
  const root = window.document.documentElement;

  assert.equal(root.classList.contains('dark'), true);
  assert.equal(root.style.colorScheme, 'dark');
  assert.equal(root.dataset.askoloMode, 'dark');
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