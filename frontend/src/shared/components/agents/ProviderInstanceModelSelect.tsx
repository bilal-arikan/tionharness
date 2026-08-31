// ProviderInstanceModelSelect picks a provider INSTANCE (not a kind) for an
// agent, driven by GET /api/providers + /api/provider-kinds (_Docs/71 Faz 4).
// The model list is derived from the selected instance's kind manifest, with
// the instance's own `models` override taking precedence when set. No kind id
// is hard-coded here — everything renders from the fetched kind/instance data.
import { useMemo, useState } from 'react'
import { useProviderInstances } from '@/features/settings/providers/useProviderInstances'
import type { CatalogModelInfo } from '@/api/providers'

// Sentinel <option> value for "the user has not picked a model yet". It must
// differ from '' — that is the catalog's real "session model" entry.
const UNSELECTED = '__unselected__'

const inputCls =
  'w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]'

function formatContextWindow(tokens?: number): string {
  if (!tokens || tokens <= 0) return ''
  if (tokens >= 1_000_000) {
    const m = tokens / 1_000_000
    return `${Number.isInteger(m) ? m : m.toFixed(1)}M`
  }
  return `${Math.round(tokens / 1000)}K`
}

// parseModelOverride mirrors the backend's parseModelList (comma/newline
// separated id list, label = id).
function parseModelOverride(s: string): CatalogModelInfo[] {
  return s
    .split(/[,\n\r]/)
    .map((f) => f.trim())
    .filter(Boolean)
    .map((id) => ({ id, label: id }))
}

interface Props {
  /** The agent's current provider INSTANCE id (Agent.providerInstanceId). */
  providerInstanceId: string
  /** The selected model id. `''` is a REAL choice — the catalog's ID:"" entry
   * ("… oturum modeli"), which lets the CLI pick the model. `null` means the
   * user has not chosen yet: the select shows a placeholder and the caller is
   * expected to block submit. */
  model: string | null
  /** Called with (kindId, instanceId, model) whenever the selection changes —
   * the caller persists kindId as `provider` (backend derives it anyway, but
   * keeping the local form state consistent avoids a stale badge) and
   * instanceId as `providerInstanceId`. */
  onChange: (kindId: string, instanceId: string, model: string) => void
}

// ProviderInstanceModelSelect is the Faz 4 replacement for ProviderModelSelect
// in the agent form: it selects a configured provider INSTANCE (label + kind
// badge), not a bare kind id, so two instances of the same kind (e.g. two
// Anthropic accounts) are independently selectable. If the agent's stored
// instance id no longer resolves (the instance was deleted), it shows a clear
// warning instead of silently falling back to some other instance.
export function ProviderInstanceModelSelect({ providerInstanceId, model, onChange }: Props) {
  const { instances, kinds, loading } = useProviderInstances()
  const [custom, setCustom] = useState(false)

  const selected = instances.find((i) => i.id === providerInstanceId)
  const selectedKind = kinds.find((k) => k.id === selected?.kindId)
  const isOrphaned = !loading && providerInstanceId !== '' && !selected

  // Model list: the instance's own override (if set) wins, otherwise the
  // kind's curated manifest list.
  const models = useMemo<CatalogModelInfo[]>(() => {
    if (selected?.models) return parseModelOverride(selected.models)
    return selectedKind?.models ?? []
  }, [selected, selectedKind])

  // `null` = nothing picked yet. It is neither a list entry nor a custom id, so
  // it must short-circuit both checks below.
  const unselected = model === null
  const inList = !unselected && models.some((m) => m.id === model)
  const showCustom = custom || (!unselected && !inList && model !== '' && !!selected)
  const selectedModelInfo = unselected ? undefined : models.find((m) => m.id === model)

  const selectInstance = (instanceId: string) => {
    const inst = instances.find((i) => i.id === instanceId)
    if (!inst) return
    const nextModels = inst.models
      ? parseModelOverride(inst.models)
      : (kinds.find((k) => k.id === inst.kindId)?.models ?? [])
    onChange(inst.kindId, inst.id, inst.defaultModel || nextModels[0]?.id || '')
    setCustom(false)
  }

  // Group instances by kind so same-kind instances are visually distinguishable
  // via an <optgroup> label, then each option carries its own label + id.
  const kindLabel = (kindId: string) => kinds.find((k) => k.id === kindId)?.label ?? kindId
  const byKind = useMemo(() => {
    const groups = new Map<string, typeof instances>()
    for (const inst of instances) {
      const list = groups.get(inst.kindId) ?? []
      list.push(inst)
      groups.set(inst.kindId, list)
    }
    return groups
  }, [instances])

  return (
    <div className="space-y-2">
      <div className="grid grid-cols-2 gap-3">
        <label className="block space-y-1">
          <span className="text-xs font-medium text-[var(--color-text-dim)]">Sağlayıcı</span>
          <select
            data-testid="provider-instance-select"
            value={providerInstanceId}
            onChange={(e) => selectInstance(e.target.value)}
            className={inputCls}
          >
            {isOrphaned && (
              <option value={providerInstanceId}>{providerInstanceId} (silinmiş örnek)</option>
            )}
            {[...byKind.entries()].map(([kindId, list]) => (
              <optgroup key={kindId} label={kindLabel(kindId)}>
                {list.map((inst) => (
                  <option key={inst.id} value={inst.id}>
                    {inst.label || inst.id}
                    {!inst.enabled ? ' (devre dışı)' : ''}
                  </option>
                ))}
              </optgroup>
            ))}
          </select>
        </label>

        <label className="block space-y-1">
          <span className="text-xs font-medium text-[var(--color-text-dim)]">Model</span>
          {showCustom ? (
            <div className="flex gap-1">
              <input
                data-testid="model-custom-input"
                value={model ?? ''}
                onChange={(e) =>
                  onChange(selected?.kindId ?? '', providerInstanceId, e.target.value)
                }
                placeholder="model adı"
                className={inputCls}
              />
              {selected && (
                <button
                  type="button"
                  data-testid="model-reset-to-list"
                  onClick={() => {
                    setCustom(false)
                    onChange(selected.kindId, providerInstanceId, models[0]?.id ?? '')
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
              data-testid="model-select"
              value={unselected ? UNSELECTED : model}
              onChange={(e) => {
                if (e.target.value === '__custom__') {
                  setCustom(true)
                } else if (e.target.value !== UNSELECTED) {
                  onChange(selected?.kindId ?? '', providerInstanceId, e.target.value)
                }
              }}
              className={inputCls}
              disabled={isOrphaned}
            >
              {unselected && (
                <option value={UNSELECTED} disabled>
                  Model seçin…
                </option>
              )}
              {models.map((m) => {
                const w = formatContextWindow(m.contextWindow)
                return (
                  <option key={m.id || '__default__'} value={m.id}>
                    {m.label ?? m.id}
                    {w ? ` · ${w}` : ''}
                  </option>
                )
              })}
              <option value="__custom__">Özel…</option>
            </select>
          )}
          {!showCustom && selectedModelInfo?.description && (
            <span className="block text-xs text-[var(--color-text-dim)]">
              {selectedModelInfo.description}
            </span>
          )}
        </label>
      </div>

      {isOrphaned && (
        <p
          data-testid="provider-instance-orphaned-warning"
          className="rounded-md border border-[color-mix(in_srgb,var(--color-danger)_45%,transparent)] bg-[color-mix(in_srgb,var(--color-danger)_10%,transparent)] px-2 py-1.5 text-xs text-[var(--color-danger)]"
        >
          ⚠ Bu ajanın bağlı olduğu sağlayıcı örneği ("{providerInstanceId}") silinmiş. Ajan bu
          haliyle çalışmaz — yukarıdan yeni bir sağlayıcı örneği seçip kaydedin.
        </p>
      )}
    </div>
  )
}
