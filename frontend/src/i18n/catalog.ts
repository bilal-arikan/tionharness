// Catalog loading. Translation files live at
//   src/i18n/locales/<locale>/<namespace>.json
// and are collected here with Vite's import.meta.glob so adding a namespace is a
// matter of dropping in a file — no registration list to keep in sync.
//
// Loading is EAGER on purpose. The catalogs are plain text bundled into the same
// single binary that serves the SPA, and a local-first app has no network round
// trip to amortise, so lazy namespace loading would buy a few dozen kilobytes at
// the cost of an async boot and a flash of untranslated chrome. If the catalogs
// ever grow past a few hundred KB, switch the glob below to `eager: false` and
// wire i18next's backend plugin — nothing else here depends on the timing.

import { LOCALE_CODES, type Locale } from './locales'

type Catalog = Record<string, unknown>
type LocaleResources = Record<string, Catalog>

const modules = import.meta.glob<Catalog>('./locales/*/*.json', {
  eager: true,
  import: 'default',
})

// NAMESPACE_SEP mirrors i18next's default ':' separator (t('chat:composer.send')).
export const NAMESPACE_SEP = ':'

// DEFAULT_NAMESPACE holds strings shared across features (actions, states, units).
// Feature-local strings belong in a namespace named after the feature folder.
export const DEFAULT_NAMESPACE = 'common'

// parsePath turns './locales/en/chat.json' into ['en', 'chat'], or null when the
// file does not sit at the expected depth.
function parsePath(path: string): [string, string] | null {
  const m = /\.\/locales\/([^/]+)\/([^/]+)\.json$/.exec(path)
  return m ? [m[1], m[2]] : null
}

// buildResources groups every catalog file into the nested shape i18next expects:
// { en: { common: {...}, chat: {...} }, tr: { ... } }.
export function buildResources(): Record<string, LocaleResources> {
  const out: Record<string, LocaleResources> = {}
  for (const [path, catalog] of Object.entries(modules)) {
    const parsed = parsePath(path)
    if (!parsed) continue
    const [locale, ns] = parsed
    ;(out[locale] ??= {})[ns] = catalog
  }
  return out
}

// namespacesOf lists the namespaces present for a locale. Used by the parity test
// and by the i18next init to declare the namespace set up front.
export function namespacesOf(resources: Record<string, LocaleResources>, locale: string): string[] {
  return Object.keys(resources[locale] ?? {}).sort()
}

// allNamespaces is the union of namespaces across every locale, so a namespace
// that exists only in the source language is still declared (and therefore still
// resolvable through the fallback chain).
export function allNamespaces(resources: Record<string, LocaleResources>): string[] {
  const set = new Set<string>()
  for (const code of LOCALE_CODES) {
    for (const ns of namespacesOf(resources, code)) set.add(ns)
  }
  set.add(DEFAULT_NAMESPACE)
  return [...set].sort()
}

export type { Catalog, LocaleResources, Locale }
