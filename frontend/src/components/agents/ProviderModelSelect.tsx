import { useEffect, useState } from 'react'
import { api } from '../../api'
import type { CatalogEntry } from '../../types'

// Module-level cache so the catalog is fetched once across all pickers.
let catalogCache: CatalogEntry[] | null = null
let catalogPromise: Promise<CatalogEntry[]> | null = null
function loadCatalog(): Promise<CatalogEntry[]> {
  if (catalogCache) return Promise.resolve(catalogCache)
  if (!catalogPromise) {
    catalogPromise = api.getCatalog().then((c) => {
      catalogCache = c
      return c
    })
  }
  return catalogPromise
}

const inputCls =
  'w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]'

interface Props {
  provider: string
  model: string
  onChange: (provider: string, model: string) => void
}

// ProviderModelSelect is a reusable provider + model picker driven by the
// backend catalog. The model field is a list of curated models with an "Özel…"
// escape hatch for typing a custom id (model ids change often).
export function ProviderModelSelect({ provider, model, onChange }: Props) {
  const [catalog, setCatalog] = useState<CatalogEntry[]>(catalogCache ?? [])
  const [custom, setCustom] = useState(false)

  useEffect(() => {
    loadCatalog().then(setCatalog).catch(() => {})
  }, [])

  const entry = catalog.find((c) => c.id === provider)
  const models = entry?.models ?? []
  const inList = models.some((m) => m.id === model)
  const showCustom = custom || (!inList && model !== '' && !!entry)
  const selectedDesc = models.find((m) => m.id === model)?.description

  const selectProvider = (p: string) => {
    const e = catalog.find((c) => c.id === p)
    onChange(p, e?.models[0]?.id ?? '')
    setCustom(false)
  }

  return (
    <div className="grid grid-cols-2 gap-3">
      <label className="block space-y-1">
        <span className="text-xs font-medium text-[var(--color-text-dim)]">Sağlayıcı</span>
        <select value={provider} onChange={(e) => selectProvider(e.target.value)} className={inputCls}>
          {!entry && <option value={provider}>{provider || '(seç)'}</option>}
          {catalog.map((c) => (
            <option key={c.id} value={c.id}>
              {c.label}
              {c.needsKey && !c.available ? ' — anahtar gerek' : ''}
            </option>
          ))}
        </select>
      </label>

      <label className="block space-y-1">
        <span className="text-xs font-medium text-[var(--color-text-dim)]">Model</span>
        {showCustom ? (
          <div className="flex gap-1">
            <input
              value={model}
              onChange={(e) => onChange(provider, e.target.value)}
              placeholder="model adı"
              className={inputCls}
            />
            {entry && (
              <button
                type="button"
                onClick={() => {
                  setCustom(false)
                  onChange(provider, models[0]?.id ?? '')
                }}
                title="Listeden seç"
                className="shrink-0 rounded border border-[var(--color-border)] px-2 text-xs text-[var(--color-text-dim)] hover:border-[var(--color-accent)]"
              >
                ↩
              </button>
            )}
          </div>
        ) : (
          <select
            value={model}
            onChange={(e) => {
              if (e.target.value === '__custom__') {
                setCustom(true)
              } else {
                onChange(provider, e.target.value)
              }
            }}
            className={inputCls}
          >
            {models.map((m) => (
              <option key={m.id || '__default__'} value={m.id}>
                {m.label}
              </option>
            ))}
            {(entry?.allowCustomModel ?? true) && <option value="__custom__">Özel…</option>}
          </select>
        )}
        {!showCustom && selectedDesc && (
          <span className="block text-xs text-[var(--color-text-dim)]">{selectedDesc}</span>
        )}
      </label>
    </div>
  )
}
