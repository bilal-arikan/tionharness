// Catalog integrity guard.
//
// The whole point of the i18n layer is that a key resolves in every language. The
// failure mode it protects against is silent: a key added to the source catalog
// and forgotten in the others renders English text inside an otherwise Turkish
// screen, which nobody notices until a user reports it. These tests turn that
// into a red build instead.

import { describe, expect, it } from 'vitest'
import { allNamespaces, buildResources, namespacesOf } from './catalog'
import { FALLBACK_LOCALE, LOCALE_CODES, LOCALES } from './locales'

const resources = buildResources()

// flatten turns a nested catalog into dotted leaf paths ("time.bucket.today"), so
// two catalogs can be compared as flat key sets regardless of nesting.
function flatten(obj: unknown, prefix = ''): string[] {
  if (obj === null || typeof obj !== 'object') return [prefix]
  return Object.entries(obj as Record<string, unknown>).flatMap(([k, v]) =>
    flatten(v, prefix ? `${prefix}.${k}` : k),
  )
}

function keysOf(locale: string, ns: string): string[] {
  const catalog = (resources[locale] ?? {})[ns]
  return catalog ? flatten(catalog).sort() : []
}

describe('locale registry', () => {
  it('has a catalog directory for every registered locale', () => {
    for (const meta of LOCALES) {
      expect(
        namespacesOf(resources, meta.code).length,
        `locale "${meta.code}" is registered in locales.ts but has no catalog files under i18n/locales/${meta.code}/`,
      ).toBeGreaterThan(0)
    }
  })

  it('has no catalog directory for an unregistered locale', () => {
    for (const dir of Object.keys(resources)) {
      expect(
        LOCALE_CODES,
        `catalog directory "${dir}" has no matching entry in locales.ts`,
      ).toContain(dir)
    }
  })
})

describe('catalog parity', () => {
  const namespaces = allNamespaces(resources)

  it('defines the same namespaces in every locale', () => {
    const source = namespacesOf(resources, FALLBACK_LOCALE)
    for (const code of LOCALE_CODES) {
      expect(namespacesOf(resources, code), `namespace drift in "${code}"`).toEqual(source)
    }
  })

  it.each(namespaces)('namespace "%s" has the same keys in every locale', (ns) => {
    const source = keysOf(FALLBACK_LOCALE, ns)
    expect(source.length, `source catalog ${FALLBACK_LOCALE}/${ns}.json is empty`).toBeGreaterThan(
      0,
    )
    for (const code of LOCALE_CODES) {
      if (code === FALLBACK_LOCALE) continue
      const missing = source.filter((k) => !keysOf(code, ns).includes(k))
      const extra = keysOf(code, ns).filter((k) => !source.includes(k))
      expect(missing, `${code}/${ns}.json is missing keys`).toEqual([])
      expect(extra, `${code}/${ns}.json has keys absent from the source language`).toEqual([])
    }
  })

  it.each(namespaces)('namespace "%s" has no empty translations', (ns) => {
    for (const code of LOCALE_CODES) {
      const catalog = (resources[code] ?? {})[ns] ?? {}
      const blanks = flatten(catalog).filter((path) => {
        const value = path
          .split('.')
          .reduce<unknown>((acc, k) => (acc as Record<string, unknown>)?.[k], catalog)
        return typeof value === 'string' && value.trim() === ''
      })
      expect(blanks, `${code}/${ns}.json has blank values`).toEqual([])
    }
  })

  // Interpolation placeholders are part of the contract between the code and the
  // catalog: dropping {{count}} while translating produces a label that silently
  // loses its number rather than throwing.
  it.each(namespaces)('namespace "%s" keeps the same placeholders per key', (ns) => {
    const placeholders = (s: unknown) =>
      typeof s === 'string' ? [...s.matchAll(/\{\{(\w+)/g)].map((m) => m[1]).sort() : []
    const read = (catalog: unknown, path: string) =>
      path.split('.').reduce<unknown>((acc, k) => (acc as Record<string, unknown>)?.[k], catalog)

    const sourceCatalog = (resources[FALLBACK_LOCALE] ?? {})[ns] ?? {}
    for (const path of keysOf(FALLBACK_LOCALE, ns)) {
      const want = placeholders(read(sourceCatalog, path))
      for (const code of LOCALE_CODES) {
        if (code === FALLBACK_LOCALE) continue
        const got = placeholders(read((resources[code] ?? {})[ns] ?? {}, path))
        expect(got, `${code}/${ns}.json:${path} placeholder mismatch`).toEqual(want)
      }
    }
  })
})
