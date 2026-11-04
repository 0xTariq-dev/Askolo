import { execFileSync } from 'node:child_process';
import { readFile, writeFile } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

interface ThemeColor {
  $value: string;
}

interface ThemeTokens {
  themePresets: Record<string, Record<'light' | 'dark', Record<string, ThemeColor>>>;
}

const directory = dirname(fileURLToPath(import.meta.url));
const publicDirectory = resolve(directory, '../../public');
const sourceImage = resolve(publicDirectory, 'icons/logo-hd.png');
const displayLogoPath = resolve(publicDirectory, 'icons/logo-display.png');
execFileSync(
  'magick',
  [
    sourceImage,
    '-resize',
    '512x512',
    '-strip',
    '-define',
    'png:compression-level=9',
    displayLogoPath,
  ],
  { stdio: 'inherit' },
);
const logoData = (await readFile(displayLogoPath)).toString('base64');
const embeddedLogoSvg = (title: string) =>
  `<svg xmlns="http://www.w3.org/2000/svg" width="512" height="512" viewBox="0 0 512 512">\n` +
  `  <title>${title}</title>\n` +
  `  <image x="0" y="0" width="512" height="512" href="data:image/png;base64,${logoData}" />\n` +
  `</svg>\n`;
await Promise.all([
  writeFile(resolve(publicDirectory, 'icons/logo.svg'), embeddedLogoSvg('Askolo')),
  writeFile(resolve(publicDirectory, 'icons/fav.svg'), embeddedLogoSvg('Askolo favicon')),
]);
const tokens = JSON.parse(
  await readFile(resolve(directory, 'askolo-design-tokens.json'), 'utf8'),
) as ThemeTokens;
const sourceLogoHue = 189;
const themeIds = ['blueHorizon', 'warmPaper', 'neutral'];
const modes = ['light', 'dark'] as const;

function hueFromHex(hex: string): number {
  const match = /^#([0-9a-f]{6})$/i.exec(hex);
  if (!match) throw new Error(`Expected a six-digit theme color, received "${hex}".`);

  const [red, green, blue] = match[1]
    .match(/.{2}/g)!
    .map((channel) => Number.parseInt(channel, 16) / 255);
  const max = Math.max(red, green, blue);
  const min = Math.min(red, green, blue);
  const delta = max - min;
  if (delta === 0) return 0;

  let hue: number;
  if (max === red) hue = (green - blue) / delta + (green < blue ? 6 : 0);
  else if (max === green) hue = (blue - red) / delta + 2;
  else hue = (red - green) / delta + 4;
  return hue * 60;
}

function imageColorArguments(themeId: string, primary: string): string[] {
  if (themeId === 'neutral') return ['-colorspace', 'Gray', '-colorspace', 'sRGB'];

  const difference = ((hueFromHex(primary) - sourceLogoHue + 540) % 360) - 180;
  const huePercent = 100 + difference / 1.8;
  return [
    '-define',
    'modulate:colorspace=hsl',
    '-modulate',
    `100,100,${huePercent.toFixed(3)}`,
  ];
}

for (const themeId of themeIds) {
  const theme = tokens.themePresets[themeId];
  if (!theme) throw new Error(`Missing theme tokens for "${themeId}".`);

  for (const mode of modes) {
    const primary = theme[mode].primary?.$value;
    if (!primary) throw new Error(`Missing ${mode} primary color for "${themeId}".`);

    for (const [kind, size] of [
      ['fav', 64],
      ['touch', 180],
    ] as const) {
      const output = resolve(publicDirectory, 'icons', `${kind}-${themeId}-${mode}.png`);
      execFileSync(
        'magick',
        [
          sourceImage,
          ...imageColorArguments(themeId, primary),
          '-resize',
          `${size}x${size}`,
          '-gravity',
          'center',
          '-background',
          'none',
          '-extent',
          `${size}x${size}`,
          output,
        ],
        { stdio: 'inherit' },
      );
    }
  }
}
