import { readFile, writeFile } from 'node:fs/promises';
import { THEME_PRESETS, getThemeVariables, type ThemeId, type ThemeMode, type SurfaceStyleId } from './theme';

const themeIds: ThemeId[] = ['blueHorizon', 'warmPaper'];
const modes: ThemeMode[] = ['light', 'dark'];
const surfaceStyles: SurfaceStyleId[] = ['soft', 'tinted'];
const blocks = themeIds.flatMap((themeId) =>
  modes.flatMap((mode) =>
    surfaceStyles.map((surfaceStyle) => {
      const variables = getThemeVariables(themeId, mode, surfaceStyle);
      const declarations = Object.entries(variables)
        .map(([name, value]) => `  ${name}: ${value};`)
        .join('\n');
      return `html[data-askolo-theme="${themeId}"][data-askolo-mode="${mode}"][data-askolo-surface-style="${surfaceStyle}"] {\n${declarations}\n}`;
    }),
  ),
);

const css = [
  '/* Generated from askolo-design-tokens.json by build-theme-token-fallbacks.ts. */',
  '@layer base {',
  ...blocks,
  '}',
  '',
].join('\n\n');

await writeFile(new URL('../theme-token-fallbacks.css', import.meta.url), css);
await writeFile(
  new URL('../../public/askolo-theme-token-fallbacks.css', import.meta.url),
  css,
);
await writeFile(
  new URL('../../public/askolo-fonts.css', import.meta.url),
  await readFile(new URL('./askolo-fonts.css', import.meta.url), 'utf8'),
);

if (Object.keys(THEME_PRESETS).length !== themeIds.length) {
  throw new Error('The local theme selector and fallback generator support different preset sets.');
}