import { useEffect, useState } from 'react'
import { api } from '@/api'
import type { CatalogEntry } from '@/types'

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
    loadCatalog().then(setCatalog).catch(() => {})
  }, [])
  return catalog
}

// stripTagline drops the descriptive suffix catalog labels append after a
// space-delimited dash ("Sonnet — dengeli" → "Sonnet", "MiniMax M3 - guncel
// amiral" → "MiniMax M3"). The dash must be surrounded by spaces so in-name
// hyphens ("GPT-5.5 Pro") and parenthetical variants ("Opus 4.8 (Fast)") are
// left intact. Full labels with taglines stay in the model pickers; only the
// agent-adjacent display (this resolver) shows the bare name.
function stripTagline(label: string): string {
  return label.replace(/\s+[—–-]\s+.*$/, '').trim()
}

// resolveModelLabel returns a human label for an agent's effective model: the
// configured model's catalog label (or its raw id when it's a custom value), or
// the provider's default model label when the agent left the model empty (e.g.
// claude-cli's "Varsayılan (oturum modeli)"). Falls back to the raw model or
// provider string when the catalog isn't loaded yet.
export function resolveModelLabel(
  catalog: CatalogEntry[],
  provider: string,
  model: string,
): string {
  const entry = catalog.find((c) => c.id === provider)
  if (!entry) return model || provider
  const exact = entry.models.find((m) => m.id === model)
  if (exact) return stripTagline(exact.label || exact.id || provider)
  if (model) return model // a custom model id not present in the curated list
  const def = entry.models[0]
  return stripTagline(def?.label || def?.id || provider)
}
