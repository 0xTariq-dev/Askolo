import { defineConfig } from '@lingui/cli';
import { formatter } from '@lingui/format-po';

export default defineConfig({
  sourceLocale: 'en',
  locales: ['en', 'ar'],
  fallbackLocales: { default: 'en' },
  catalogs: [
    {
      path: '<rootDir>/locales/{locale}/messages',
      include: ['<rootDir>/src'],
      exclude: ['<rootDir>/src/**/*.d.ts'],
    },
  ],
  format: formatter(),
});