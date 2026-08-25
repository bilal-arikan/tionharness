// Locale registry — the single source of truth for which languages the UI can
// render in. Adding a language is a three-step change:
//   1. append an entry here,
//   2. create frontend/src/i18n/locales/<code>/ with the catalog files,
//   3. append the code to SupportedLanguages in internal/settings/language.go.
//
// The catalog-parity test (catalog.test.ts) fails until step 2 matches step 1,
// so a half-added language cannot ship.

export type Locale = 'tr' | 'en'

export interface LocaleMeta {
  code: Locale
  // label is intentionally written in the language itself (endonym): a user
  // looking for their language should recognise it without already reading the
  // current UI language.
  label: string
  // intl is the BCP-47 tag handed to Intl.* formatters and Intl.Collator. It is
  // separate from `code` because a UI language may map to a region-qualified
  // formatting locale (e.g. 'tr' → 'tr-TR' for the 24-hour clock and dotted
  // thousands separator).
  intl: string
  // dir drives the document's writing direction. Every current locale is LTR;
  // the field exists so adding Arabic/Hebrew later is a data change rather than
  // a plumbing change.
  dir: 'ltr' | 'rtl'
}

export const LOCALES: readonly LocaleMeta[] = [
  { code: 'tr', label: 'Türkçe', intl: 'tr-TR', dir: 'ltr' },
  { code: 'en', label: 'English', intl: 'en-US', dir: 'ltr' },
] as const

// DEFAULT_LOCALE mirrors settings.DefaultLanguage on the backend: what the UI
// renders in before any preference has been read.
export const DEFAULT_LOCALE: Locale = 'tr'

// FALLBACK_LOCALE is where i18next looks when a key is missing from the active
// catalog. English is the source language (see _Docs/73), so a not-yet-translated
// key degrades to English rather than to a raw key path.
export const FALLBACK_LOCALE: Locale = 'en'

export const LOCALE_CODES: readonly Locale[] = LOCALES.map((l) => l.code)

// isLocale narrows an arbitrary string to a supported locale code.
export function isLocale(v: unknown): v is Locale {
  return typeof v === 'string' && (LOCALE_CODES as readonly string[]).includes(v)
}

// localeMeta returns the registry entry for a code, falling back to the default
// so callers never have to handle a null.
export function localeMeta(code: string): LocaleMeta {
  return LOCALES.find((l) => l.code === code) ?? LOCALES.find((l) => l.code === DEFAULT_LOCALE)!
}

// intlTag maps a UI locale code to the tag used for date/number formatting.
export function intlTag(code: string): string {
  return localeMeta(code).intl
}
