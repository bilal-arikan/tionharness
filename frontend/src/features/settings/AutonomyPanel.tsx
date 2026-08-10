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
          onChange={(e) => {
            // Switching provider drops the model: a model id is only meaningful
            // for the provider it came from, and silently carrying e.g. an
            // Anthropic id over to OpenRouter produces a request that only fails
            // at title time.
            set('titleProviderId', e.target.value)
            set('titleModel', '')
          }}
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
      {/* The model field is shown even with no provider override. titleModel stays
          in effect on its own (it is applied to whichever agent does the titling),
          so hiding the input would leave a live setting invisible and uneditable. */}
      {selectedProvider ? (
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
            // value is the stored setting VERBATIM: falling back to models[0]
            // would paint a concrete model as selected while the saved value is
            // still "" (provider default), so the form would lie about state.
            <select
              value={draft.titleModel || ''}
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
      ) : (
        <Field
          label="Başlık modeli"
          hint="Boş = ajanın kendi modeli. Sağlayıcı seçmeden de ucuz bir model adı yazabilirsin (örn. haiku)."
        >
          <input
            value={draft.titleModel || ''}
            onChange={(e) => set('titleModel', e.target.value)}
            placeholder="örn. haiku"
            className={inputCls}
          />
        </Field>
      )}
    </>
  )
}
