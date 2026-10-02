import assert from 'node:assert/strict';
import test from 'node:test';
import { setupI18n } from '@lingui/core';
import {
  DEFAULT_LOCALE,
  isLocale,
  isLocaleCode,
  LOCALE_REGISTRY,
  SUPPORTED_LOCALES,
  directionForLocale,
  formatDate,
  formatNumber,
  formatRelativeDate,
  getStoredLocale,
  loadLocaleCatalog,
  mergeCatalogWithFallback,
} from './locale.ts';

test('locale contract defaults safely and maps Arabic to RTL', () => {
  assert.equal(getStoredLocale({ getItem: () => null }), 'en');
  assert.equal(getStoredLocale({ getItem: () => 'ar' }), 'ar');
  assert.equal(getStoredLocale({ getItem: () => 'fr' }), 'en');
  assert.equal(DEFAULT_LOCALE, 'en');
  assert.deepEqual(SUPPORTED_LOCALES, ['en', 'ar']);
  assert.equal(isLocaleCode('fr'), true);
  assert.equal(isLocaleCode('FR'), false);
  assert.equal(isLocaleCode('pt-BR'), false);
  assert.equal(isLocaleCode('e1'), false);
  assert.equal(isLocale('fr'), false);
  assert.equal(isLocale('ar'), true);
  assert.equal(directionForLocale('ar'), 'rtl');
  assert.equal(directionForLocale('en'), 'ltr');
  assert.equal(LOCALE_REGISTRY.en.displayName, 'English');
  assert.equal(LOCALE_REGISTRY.en.direction, 'ltr');
  assert.equal(LOCALE_REGISTRY.ar.direction, 'rtl');
  assert.equal(LOCALE_REGISTRY.en.weekStartsOn, 1);
  assert.equal(LOCALE_REGISTRY.ar.weekStartsOn, 1);
  assert.equal(LOCALE_REGISTRY.en.dateFnsLocale.code, 'en-US');
  assert.equal(LOCALE_REGISTRY.ar.dateFnsLocale.code, 'ar-SA');
  assert.equal(LOCALE_REGISTRY.en.dateFnsLocaleCode, LOCALE_REGISTRY.en.dateFnsLocale.code);
  assert.equal(LOCALE_REGISTRY.ar.dateFnsLocaleCode, LOCALE_REGISTRY.ar.dateFnsLocale.code);
});

test('missing localized catalog entries inherit English before rendering', () => {
  const resolved = mergeCatalogWithFallback(
    { 'profile.account': 'Account', 'calendar.today': 'Today' },
    { 'calendar.today': 'اليوم' },
  );
  assert.equal(resolved['profile.account'], 'Account');
  assert.equal(resolved['calendar.today'], 'اليوم');
});

test('catalog loading merges Arabic with English and falls back when its chunk fails', async () => {
  const english = { 'profile.account': 'Account', 'calendar.today': 'Today' };
  const loaders = {
    en: async () => english,
    ar: async () => ({ 'calendar.today': 'اليوم' }),
  };
  assert.deepEqual(await loadLocaleCatalog('ar', loaders), {
    'profile.account': 'Account',
    'calendar.today': 'اليوم',
  });

  const failedArabicLoaders = {
    en: async () => english,
    ar: async () => {
      throw new Error('catalog unavailable');
    },
  };
  assert.deepEqual(await loadLocaleCatalog('ar', failedArabicLoaders), english);
  await assert.rejects(loadLocaleCatalog('en', { en: async () => { throw new Error('english missing'); } }));
});

test('Lingui interpolates and pluralizes messages using the active locale', () => {
  const i18n = setupI18n();
  i18n.load({
    en: {
      'calendar.lastSynced': 'Last synced {time}',
      'calendar.moreEvents':
        '{count, plural, one {{formattedCount} more event} other {{formattedCount} more events}}',
    },
    ar: {
      'calendar.lastSynced': 'آخر مزامنة: {time}',
      'calendar.moreEvents':
        '{count, plural, zero {لا توجد أحداث إضافية} one {حدث إضافي واحد} two {حدثان إضافيان} few {{formattedCount} أحداث إضافية} many {{formattedCount} حدثًا إضافيًا} other {{formattedCount} حدث إضافي}}',
    },
  });

  i18n.activate('en');
  assert.equal(i18n._('calendar.moreEvents', { count: 1, formattedCount: '1' }), '1 more event');
  assert.equal(i18n._('calendar.moreEvents', { count: 3, formattedCount: '3' }), '3 more events');

  i18n.activate('ar');
  assert.equal(i18n._('calendar.lastSynced', { time: 'قبل دقيقة' }), 'آخر مزامنة: قبل دقيقة');
  assert.equal(i18n._('calendar.moreEvents', { count: 2, formattedCount: '٢' }), 'حدثان إضافيان');
  assert.equal(i18n._('calendar.moreEvents', { count: 7, formattedCount: '٧' }), '٧ أحداث إضافية');
});

test('number and date formatting follow the active locale and keep Gregorian dates', () => {
  assert.equal(formatNumber(1234, 'en'), '1,234');
  assert.match(formatNumber(1234, 'ar'), /١/);
  assert.equal(
    formatDate(new Date('2026-09-29T12:00:00Z'), 'en', {
      year: 'numeric',
      month: 'long',
      day: 'numeric',
      timeZone: 'UTC',
    }),
    'September 29, 2026',
  );
  assert.match(
    formatDate(new Date('2026-09-29T12:00:00Z'), 'ar', {
      year: 'numeric',
      month: 'long',
      day: 'numeric',
      timeZone: 'UTC',
    }),
    /٢٠٢٦/,
  );
  assert.equal(
    formatRelativeDate(new Date('2026-09-28T12:00:00Z'), 'en', new Date('2026-09-29T12:00:00Z')),
    'yesterday',
  );
});