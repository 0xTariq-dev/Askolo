import assert from 'node:assert/strict';
import test from 'node:test';
import { setupI18n } from '@lingui/core';
import {
  directionForLocale,
  formatDate,
  formatNumber,
  formatRelativeDate,
  getStoredLocale,
  mergeCatalogWithFallback,
} from './locale.ts';

test('locale contract defaults safely and maps Arabic to RTL', () => {
  assert.equal(getStoredLocale({ getItem: () => null }), 'en');
  assert.equal(getStoredLocale({ getItem: () => 'ar' }), 'ar');
  assert.equal(getStoredLocale({ getItem: () => 'fr' }), 'en');
  assert.equal(directionForLocale('ar'), 'rtl');
  assert.equal(directionForLocale('en'), 'ltr');
});

test('missing localized catalog entries inherit English before rendering', () => {
  const resolved = mergeCatalogWithFallback(
    { 'profile.account': 'Account', 'calendar.today': 'Today' },
    { 'calendar.today': 'اليوم' },
  );
  assert.equal(resolved['profile.account'], 'Account');
  assert.equal(resolved['calendar.today'], 'اليوم');
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