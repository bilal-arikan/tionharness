import { useEffect, useState } from 'react'
import { Field, Toggle, inputCls } from './primitives'
import type { PanelProps } from './settingsPanelShared'
import { api } from '@/api'
import type { CatalogEntry, CatalogModel } from '@/types'

// modelDisplayName shows a short label for a model, appending the resolved
// concrete id when it differs from the entry's model id (aliases like "sonnet").
function modelDisplayName(m: CatalogModel): string {
  if (m.resolvedModel && m.resolvedModel !== m.id) {
    return `${m.label} (${m.resolvedModel})`
  }
  return m.label
}

export function AutoTitlePanel({ draft, set }: PanelProps) {
  const [catalog, setCatalog] = useState<CatalogEntry[]>([])
  const [loadingCatalog, setLoadingCatalog] = useState(true)

  useEffect(() => {
    api
      .getCatalog()
      .then(setCatalog)
      .catch(() => {})
      .finally(() => setLoadingCatalog(false))
  }, [])

  const available = catalog.filter((c) => c.available)
  const selectedProvider = available.find((c) => c.id === draft.titleProviderId)
  return (
    <>
      <Toggle
        label="Otomatik başlık üretimi"
        hint="Sohbet ilk mesajında ve görev oluşturmada başlık otomatik üretilir."
        checked={draft.autoTitleEnabled}
        onChange={(v) => set('autoTitleEnabled', v)}
      />
      <Field
        label="Başlık sağlayıcısı"
        hint="Boş = ajanın kendi sağlayıcısı. Başlık üretimi için ucuz bir sağlayıcı seçebilirsin."
      >
        <select
          value={draft.titleProviderId || ''}
          onChange={(e) => set('titleProviderId', e.target.value)}
          className={inputCls}
          disabled={loadingCatalog}
        >
          <option value="">Ajanın sağlayıcısı (varsayılan)</option>
          {available.map((p) => (
            <option key={p.id} value={p.id}>
              {p.label}
            </option>
          ))}
        </select>
      </Field>
      {selectedProvider && (
        <Field
          label="Başlık modeli"
          hint={
            selectedProvider.allowCustomModel
              ? 'Seç veya özel model adı yaz. Boş = sağlayıcının varsayılan modeli.'
              : 'Boş = sağlayıcının varsayılan modeli.'
          }
        >
          {selectedProvider.allowCustomModel ? (
            <input
              value={draft.titleModel || ''}
              onChange={(e) => set('titleModel', e.target.value)}
              placeholder={selectedProvider.models[0]?.id || 'model id'}
              className={inputCls}
              list="title-model-suggestions"
            />
          ) : (
            <select
              value={draft.titleModel || selectedProvider.models[0]?.id || ''}
              onChange={(e) => set('titleModel', e.target.value)}
              className={inputCls}
            >
              <option value="">Varsayılan</option>
              {selectedProvider.models.map((m) => (
                <option key={m.id} value={m.id}>
                  {modelDisplayName(m)}
                </option>
              ))}
            </select>
          )}
          {selectedProvider.models.length > 0 && (
            <datalist id="title-model-suggestions">
              {selectedProvider.models.map((m) => (
                <option key={m.id} value={m.id}>
                  {modelDisplayName(m)}
                </option>
              ))}
            </datalist>
          )}
        </Field>
      )}
    </>
  )
}
