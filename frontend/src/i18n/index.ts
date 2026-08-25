// i18next bootstrap. Importing this module initialises the translator; it is
// imported once from main.tsx before the app renders so no component can observe
// a half-initialised instance.
//
// Deliberate choices:
//   - Catalogs are bundled (see catalog.ts), so init is synchronous and there is
//     no Suspense boundary to manage.
//   - `returnNull: false` makes a missing key render its own key path instead of
//     an empty string, so a gap is visible in the UI rather than silently blank.
//   - Interpolation escaping is off because React already escapes everything it
//     renders; leaving it on would double-escape apostrophes in Turkish copy.

import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'
import { allNamespaces, buildResources, DEFAULT_NAMESPACE } from './catalog'
import { cacheLocale, initialLocale } from './bootLocale'
import { DEFAULT_LOCALE, FALLBACK_LOCALE, isLocale, localeMeta, type Locale } from './locales'

const resources = buildResources()

void i18next.use(initReactI18next).init({
  resources,
  lng: initialLocale(),
  fallbackLng: FALLBACK_LOCALE,
  supportedLngs: [...new Set([DEFAULT_LOCALE, FALLBACK_LOCALE, ...Object.keys(resources)])],
  ns: allNamespaces(resources),
  defaultNS: DEFAULT_NAMESPACE,
  returnNull: false,
  interpolation: { escapeValue: false },
  // Silence the "key not found" console noise in production; during migration the
  // untranslated surface is large and known, and the rendered key path is the
  // signal we actually act on.
  saveMissing: false,
  debug: false,
})

applyDocumentLocale(currentLocale())

// currentLocale returns the active UI locale, narrowed to a supported code.
export function currentLocale(): Locale {
  const lng = i18next.resolvedLanguage ?? i18next.language
  return isLocale(lng) ? lng : DEFAULT_LOCALE
}

// applyDocumentLocale reflects the locale onto <html> so the browser hyphenates,
// spell-checks and (for a future RTL locale) mirrors the layout correctly. CSS and
// third-party widgets read these attributes; nothing else in the app has to.
function applyDocumentLocale(locale: Locale) {
  // Guarded so the module can be imported from a plain-Node vitest run (the
  // catalog/format tests have no DOM) without pulling in jsdom.
  if (typeof document === 'undefined') return
  const meta = localeMeta(locale)
  const root = document.documentElement
  root.setAttribute('lang', meta.code)
  root.setAttribute('dir', meta.dir)
}

// setLocale switches the UI language live. Returns a promise so callers that need
// to act after every subscriber has re-rendered can await it; fire-and-forget is
// fine otherwise. A no-op when the locale is already active, which keeps the
// settings SSE echo from re-rendering the whole tree on every unrelated save.
export async function setLocale(locale: Locale): Promise<void> {
  if (currentLocale() === locale) return
  await i18next.changeLanguage(locale)
  applyDocumentLocale(locale)
  cacheLocale(locale)
}

export { i18next }
export * from './locales'
export { resolveUILocale } from './bootLocale'
