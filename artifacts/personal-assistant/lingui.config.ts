import { defineConfig } from '@lingui/cli';
import { formatter } from '@lingui/format-po';
import { readFileSync } from 'node:fs';
import path from 'node:path';

const localeRegistry = JSON.parse(
  readFileSync(path.resolve(import.meta.dirname, 'src/lib/locale-registry.json'), 'utf8'),
) as {
  sourceLocale: string;
  locales: Record<string, unknown>;
};

export default defineConfig({
  sourceLocale: localeRegistry.sourceLocale,
  locales: Object.keys(localeRegistry.locales),
  fallbackLocales: { default: localeRegistry.sourceLocale },
  catalogs: [
    {
      path: '<rootDir>/locales/{locale}/messages',
      include: ['<rootDir>/src'],
      exclude: ['<rootDir>/src/**/*.d.ts'],
    },
  ],
  format: formatter(),
});