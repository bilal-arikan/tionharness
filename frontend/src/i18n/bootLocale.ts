// Boot-time locale resolution.
//
// The authoritative UI language lives in app settings on the backend, which makes
// it follow the user across windows and devices. But that value arrives one HTTP
// round trip after the first paint, so booting straight from it would flash the
// previous language on every reload. This module keeps a device-local mirror of
// the last resolved locale, used only to paint the first frame; the backend value
// overwrites it as soon as settings load.

import { DEFAULT_LOCALE, isLocale, type Locale } from './locales'

const STORAGE_KEY = 'tionharness.uiLocale'

// cachedLocale returns the last locale this device rendered in, or null when the
// app has never run here (or storage is unavailable, e.g. a hardened webview).
export function cachedLocale(): Locale | null {
  try {
    const v = localStorage.getItem(STORAGE_KEY)
    return isLocale(v) ? v : null
  } catch {
    return null
  }
}

// cacheLocale records the active locale for the next boot. Failures are ignored:
// losing the cache costs a first-frame flash, never correctness.
export function cacheLocale(locale: Locale) {
  try {
    localStorage.setItem(STORAGE_KEY, locale)
  } catch {
    /* storage disabled — the backend value still drives the session */
  }
}

// browserLocale reads the navigator's preferred language, used only when this
// device has no cache yet. Matches on the primary subtag so 'en-GB' finds 'en'.
export function browserLocale(): Locale | null {
  const langs =
    typeof navigator === 'undefined' ? [] : (navigator.languages ?? [navigator.language])
  for (const tag of langs) {
    const primary = String(tag).split('-')[0].toLowerCase()
    if (isLocale(primary)) return primary
  }
  return null
}

// initialLocale picks what to render before settings arrive: this device's last
// locale, else the browser preference, else the app default.
export function initialLocale(): Locale {
  return cachedLocale() ?? browserLocale() ?? DEFAULT_LOCALE
}

// resolveUILocale mirrors settings.EffectiveUILanguage on the backend: an explicit
// interface language wins, otherwise the interface follows the agent reply
// language. Kept as a pure function so the settings screen can preview the
// resolved value while the user is still editing the draft.
export function resolveUILocale(uiLanguage: string, agentLanguage: string): Locale {
  if (isLocale(uiLanguage)) return uiLanguage
  if (isLocale(agentLanguage)) return agentLanguage
  return DEFAULT_LOCALE
}
