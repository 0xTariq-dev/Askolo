# Locale and RTL conventions

## Enabled locales and preference

The locale registry at `src/lib/locale-registry.json` is the shared source for
enabled locale codes, native display names, text direction, number/date
formatting, calendar, week start, date-fns locale, Lingui catalog module, and
the pre-render document bootstrap. It currently enables `en` (English, LTR)
and `ar` (Arabic, RTL). The account's `preferredLocale` is the cross-device preference and Profile is its
user-facing setting. When the account has no saved choice, the `askolo-locale`
local-storage value seeds the app and is saved to the account after sign-in.
Locale changes apply locally immediately and then sync to the account. A sync
failure keeps the current-device choice and reports the failure. Speech
transcription language is a separate provider preference and must not be used
as the app locale.

Locale codes use exactly two lowercase ASCII letters. The shared registry and
the Go profile API each explicitly enable only `en` and `ar`; a well-formed
code such as `fr` remains unsupported until it is deliberately added to both.
The database checks only the two-letter format so a future enabled locale does
not need a column or constraint redesign.

The initial document bootstrap reads the same preference before React renders.
Runtime changes update the root `lang` and `dir` together and update the mounted
React tree without changing route keys or remounting the current screen. If
storage is unavailable or contains an unsupported value, the locale defaults to
English. Changes made in another same-origin tab are applied through the storage
event.

## Catalog and fallback contract

- Catalogs use Lingui 6 gettext-style PO files at
  `locales/{locale}/messages.po`, loaded as separate Vite chunks using the
  registry's catalog module path. Keep English source descriptors in
  `src/lib/messages.ts`; run
  `pnpm --filter @workspace/personal-assistant run i18n:extract` after adding or
  changing descriptors, then add reviewed translations to the Arabic catalog.
- `fallbackLocales: { default: 'en' }` and the runtime English-catalog merge
  make untranslated Arabic entries render in English. If a message ID is absent
  from the source descriptors, the key itself is returned so missing coverage
  remains visible in testing.
- Lingui ICU messages interpolate `{name}` placeholders. Numeric interpolation
  values are formatted with the active locale before display; plural messages
  retain a raw numeric `count` for category selection and use
  `formattedCount` when the number is shown.
- Plural messages use the active locale's CLDR categories. Arabic messages
  should cover `zero`, `one`, `two`, `few`, `many`, and `other`; English
  messages normally use `one` and `other`. Always include `other`.
- Dates use `Intl.DateTimeFormat` with the registry's calendar (Gregorian for
  current locales). Numbers use `Intl.NumberFormat`; Arabic explicitly uses
  Arabic-Indic digits. Calendar week starts and date-fns locale data also come
  from the registry.
- Keep date keys, API payloads, route identifiers, and persisted enum values
  locale-neutral. Do not localize machine-readable values.

## Bidirectional layout and text

- Prefer logical layout properties and utilities (`ps`/`pe`, `ms`/`me`,
  `start`/`end`, `text-start`/`text-end`, and `border-s`/`border-e`).
- Set document `lang` and `dir` from locale state together. Do not force a
  direction on the app shell or reusable components.
- Keep DOM order, item order, and keyboard tab order stable. Let direction
  affect inline layout instead of reversing arrays or manually reordering focus.
- Mirror only directional controls such as previous/next arrows. Keep neutral
  symbols, clocks, and calendar icons unchanged.
- Use `dir="auto"` for user-authored names, titles, descriptions, and locations.
  Isolate emails, URLs, times, IDs, and other embedded LTR values with `<bdi
  dir="ltr">` or explicit `lang="en" dir="ltr"`.
- Portal overlays should inherit direction from the document root.
- Review touch gestures and physical-edge controls when an interaction depends
  on left/right; visual mirroring alone is not sufficient.

Arabic copy coverage is intentionally partial in this foundation. Untranslated
surfaces remain legible in English while inheriting Arabic reading direction.
Calendar dates remain Gregorian; local calendar systems and a complete Arabic
copy rollout are not part of this change.