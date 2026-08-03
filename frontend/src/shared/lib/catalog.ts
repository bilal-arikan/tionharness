import { useEffect, useState } from 'react'
import { api } from '@/api'
import type { CatalogEntry } from '@/types'

// Re-exported so every consumer keeps importing labels from '@/shared/lib/catalog'.
// The implementations live in modelLabel.ts (no api import → unit-testable in the
// repo's `node` vitest environment).
export { formatModelVersion, resolveModelLabel, resolveRuntimeBadge } from './modelLabel'

// Module-level cache so the provider/model catalog is fetched once and shared by
// every consumer (pickers + the agent label resolver).
let catalogCache: CatalogEntry[] | null = null
let catalogPromise: Promise<CatalogEntry[]> | null = null

export function loadCatalog(): Promise<CatalogEntry[]> {
  if (catalogCache) return Promise.resolve(catalogCache)
  if (!catalogPromise) {
    catalogPromise = api.getCatalog().then((c) => {
      catalogCache = c
      return c
    })
  }
  return catalogPromise
}

// useCatalog returns the cached catalog, loading it on first use. Starts as the
// cache (or empty) and updates once the fetch resolves.
export function useCatalog(): CatalogEntry[] {
  const [catalog, setCatalog] = useState<CatalogEntry[]>(catalogCache ?? [])
  useEffect(() => {
    loadCatalog()
      .then(setCatalog)
      .catch(() => {})
  }, [])
  return catalog
}
