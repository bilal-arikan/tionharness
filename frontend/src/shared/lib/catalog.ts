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

// ThinkingInfo pairs a model's supported reasoning tiers (null = unknown → all
// tiers enabled) with its class, so the pickers can both gate the buttons and
// explain a greyed-out one. Both come straight from the backend catalog.
export interface ThinkingInfo {
  tiers: string[] | null
  cls: string
}

// thinkingInfoForModel looks up the given provider+model in the catalog and
// returns its reasoning tiers + class. Lookup is an exact id match against the
// provider's curated model list, so bare aliases and custom typed models fall
// through to {tiers:null} — callers then leave every tier enabled and let the
// provider clamp anything the concrete model can't honour. The classification
// itself is computed backend-side (ThinkingTiersFor / ThinkingClass); this is
// just the client lookup.
export function thinkingInfoForModel(
  catalog: CatalogEntry[],
  provider: string,
  model: string,
): ThinkingInfo {
  const entry = catalog.find((c) => c.id === provider)
  const m = entry?.models.find((x) => x.id === model)
  return { tiers: m?.thinkingTiers ?? null, cls: m?.thinkingClass ?? '' }
}

// thinkingTierDisabledReason returns a short Turkish explanation for why a tier
// is inactive on a model of the given class. Called only for tiers the model
// does NOT support (absent from thinkingTiers); the class decides the wording.
export function thinkingTierDisabledReason(cls: string, tier: string): string {
  if (cls === 'always-on' && tier === 'off') return 'Bu model her zaman düşünür — kapatılamaz'
  if (cls === 'non-thinking') return 'Bu model düşünmez (akıl yürütme yok)'
  // On the reasoning classes the only upper tier a model can be missing is
  // "ultra": the Messages API effort enum stops at "max", so it lands on max
  // rather than on the legacy high clamp.
  if (tier === 'ultra' && (cls === 'adaptive' || cls === 'always-on' || cls === 'alias'))
    return 'Bu sağlayıcıda "Maks"a (max) düşer — effort değeri ultra taşımıyor'
  if (tier === 'xhigh' || tier === 'max' || tier === 'ultra')
    return 'Bu modelde "Yüksek"e (high) düşer'
  return 'Bu model bu seviyeyi desteklemez'
}

// thinkingOptionsForModel is the shared provider-aware enablement rule used by
// both the persisted agent setting and the per-turn composer picker. Keeping it
// here prevents either UI from drifting away from the backend catalog contract.
// The empty value is the composer's "Auto" option and is always selectable;
// selectedValue stays selectable so an existing setting can still be changed.
export function thinkingOptionsForModel<
  T extends { value: string; disabled?: boolean; hint?: string },
>(
  options: T[],
  catalog: CatalogEntry[],
  provider: string,
  model: string,
  selectedValue: string,
): T[] {
  const { tiers, cls } = thinkingInfoForModel(catalog, provider, model)
  if (tiers == null) return options
  return options.map((option) => {
    const supported =
      option.value === '' || option.value === selectedValue || tiers.includes(option.value)
    return supported
      ? option
      : { ...option, disabled: true, hint: thinkingTierDisabledReason(cls, option.value) }
  })
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
