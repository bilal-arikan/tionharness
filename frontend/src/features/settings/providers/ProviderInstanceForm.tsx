// ProviderInstanceForm is the generic "add/edit provider instance" form: kind
// picker (locked while editing), label, enabled toggle, default model, and
// every field the selected kind declares (config + secret), all rendered from
// the kind's manifest — no per-kind branch here.
import { useEffect, useMemo, useState } from 'react'
import { Plus } from 'lucide-react'
import type { ProviderKind, ProviderInstance, UpsertProviderInput } from '@/api/providers'
import { ProviderFieldInput } from './ProviderFieldInput'

const inputCls =
  'w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]'

interface Props {
  kinds: ProviderKind[]
  // Existing instances of each kind — used to block starting a new instance
  // of a kind whose manifest declares multi: false when one already exists.
  instances: ProviderInstance[]
  editing: ProviderInstance | null
  onCancel: () => void
  onSave: (input: UpsertProviderInput) => Promise<void>
}

function emptyDraft(kindId: string): UpsertProviderInput {
  return {
    kindId,
    label: '',
    enabled: true,
    defaultModel: '',
    models: '',
    config: {},
    secrets: {},
  }
}

export function ProviderInstanceForm({ kinds, instances, editing, onCancel, onSave }: Props) {
  const selectableKinds = useMemo(
    () =>
      kinds.filter(
        (k) => k.multi || !instances.some((i) => i.kindId === k.id) || editing?.kindId === k.id,
      ),
    [kinds, instances, editing],
  )

  const [draft, setDraft] = useState<UpsertProviderInput>(() =>
    editing
      ? {
          id: editing.id,
          kindId: editing.kindId,
          label: editing.label,
          icon: editing.icon,
          enabled: editing.enabled,
          defaultModel: editing.defaultModel,
          models: editing.models,
          config: { ...editing.config },
          secrets: {},
        }
      : emptyDraft(selectableKinds[0]?.id ?? ''),
  )
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')

  useEffect(() => {
    setDraft(
      editing
        ? {
            id: editing.id,
            kindId: editing.kindId,
            label: editing.label,
            icon: editing.icon,
            enabled: editing.enabled,
            defaultModel: editing.defaultModel,
            models: editing.models,
            config: { ...editing.config },
            secrets: {},
          }
        : emptyDraft(selectableKinds[0]?.id ?? ''),
    )
    setErr('')
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [editing])

  const kind = kinds.find((k) => k.id === draft.kindId)

  const setConfig = (key: string, value: string) =>
    setDraft((d) => ({ ...d, config: { ...d.config, [key]: value } }))
  const setSecret = (key: string, value: string) =>
    setDraft((d) => ({ ...d, secrets: { ...d.secrets, [key]: value } }))

  const save = async () => {
    setBusy(true)
    setErr('')
    try {
      await onSave(draft)
    } catch (e) {
      setErr((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  if (!editing && selectableKinds.length === 0) {
    return (
      <p className="text-xs text-[var(--color-text-dim)]">
        Tüm sağlayıcı taslakları için zaten birer örnek var (bu taslaklar çoklu örneğe izin
        vermiyor).
      </p>
    )
  }

  return (
    <div className="space-y-2 rounded-md border border-dashed border-[var(--color-border)] p-2">
      <div className="text-xs font-medium">
        {editing ? `Düzenle: ${editing.label || editing.id}` : 'Yeni sağlayıcı örneği'}
      </div>

      <div className="grid grid-cols-2 gap-1.5">
        <select
          data-testid="provider-instance-kind-select"
          value={draft.kindId}
          disabled={!!editing}
          onChange={(e) => setDraft(emptyDraft(e.target.value))}
          className={inputCls}
        >
          {(editing ? kinds : selectableKinds).map((k) => (
            <option key={k.id} value={k.id}>
              {k.label}
            </option>
          ))}
        </select>
        <input
          data-testid="provider-instance-label-input"
          placeholder="Etiket (ör. Anthropic — iş hesabı)"
          value={draft.label}
          onChange={(e) => setDraft((d) => ({ ...d, label: e.target.value }))}
          className={inputCls}
        />
      </div>

      <label className="flex items-center gap-1.5 text-xs">
        <input
          type="checkbox"
          checked={draft.enabled}
          onChange={(e) => setDraft((d) => ({ ...d, enabled: e.target.checked }))}
        />
        Etkin
      </label>

      <div className="grid grid-cols-2 gap-1.5">
        <input
          data-testid="provider-instance-default-model-input"
          placeholder="varsayılan model (opsiyonel)"
          value={draft.defaultModel}
          onChange={(e) => setDraft((d) => ({ ...d, defaultModel: e.target.value }))}
          className={inputCls}
        />
        <input
          data-testid="provider-instance-models-input"
          placeholder="model id'leri — virgülle (opsiyonel)"
          value={draft.models}
          onChange={(e) => setDraft((d) => ({ ...d, models: e.target.value }))}
          className={inputCls}
        />
      </div>

      {kind && (
        <div className="grid gap-2 sm:grid-cols-2">
          {kind.fields.map((f) =>
            f.secret ? (
              <ProviderFieldInput
                key={f.key}
                field={f}
                value={draft.secrets[f.key] ?? ''}
                isSet={!!editing?.secretsSet[f.key]}
                onChange={(v) => setSecret(f.key, v)}
              />
            ) : (
              <ProviderFieldInput
                key={f.key}
                field={f}
                value={draft.config[f.key] ?? ''}
                isSet={false}
                onChange={(v) => setConfig(f.key, v)}
              />
            ),
          )}
        </div>
      )}

      {err && <div className="text-xs text-[var(--color-danger)]">{err}</div>}

      <div className="flex gap-2">
        <button
          data-testid="provider-instance-save"
          onClick={save}
          disabled={busy || !draft.kindId || !draft.label}
          className="flex items-center gap-1 rounded bg-[var(--color-accent)] px-3 py-1.5 text-xs font-medium text-[var(--color-on-accent)] hover:opacity-90 disabled:opacity-30"
        >
          <Plus size={13} /> {editing ? 'Güncelle' : 'Ekle'}
        </button>
        <button
          data-testid="provider-instance-cancel"
          onClick={onCancel}
          className="rounded border border-[var(--color-border)] px-3 py-1.5 text-xs"
        >
          İptal
        </button>
      </div>
    </div>
  )
}
