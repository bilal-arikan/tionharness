import { useState } from 'react'
import { useCatalog, resolveRuntimeBadge } from '@/shared/lib/catalog'

const inputCls =
  'w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]'

// formatContextWindow renders a token count as a compact label (200000 → "200K",
// 1000000 → "1M"). Returns "" for unknown (0/undefined).
function formatContextWindow(tokens?: number): string {
  if (!tokens || tokens <= 0) return ''
  if (tokens >= 1_000_000) {
    const m = tokens / 1_000_000
    return `${Number.isInteger(m) ? m : m.toFixed(1)}M`
  }
  return `${Math.round(tokens / 1000)}K`
}

interface Props {
  provider: string
  model: string
  onChange: (provider: string, model: string) => void
  // When set, an extra leading provider option (value '') is offered that means
  // "inherit the app-wide default" — used by the per-workspace override form.
  allowInherit?: boolean
  inheritLabel?: string
}

// ProviderModelSelect is a reusable provider + model picker driven by the
// backend catalog. The model field is a list of curated models with an "Özel…"
// escape hatch for typing a custom id (model ids change often). With
// `allowInherit`, an empty provider is selectable and means "use the app
// default" (model becomes a free, optional text field).
export function ProviderModelSelect({
  provider,
  model,
  onChange,
  allowInherit,
  inheritLabel = '(uygulama varsayılanı)',
}: Props) {
  const catalog = useCatalog()
  const [custom, setCustom] = useState(false)

  const entry = catalog.find((c) => c.id === provider)
  const models = entry?.models ?? []
  const inList = models.some((m) => m.id === model)
  const showCustom = custom || (!inList && model !== '' && !!entry)
  const selectedModel = models.find((m) => m.id === model)
  const selectedDesc = selectedModel?.description
  const selectedWindow = formatContextWindow(selectedModel?.contextWindow)
  // claude-cli's models are aliases ("sonnet") resolved by a local Claude Code
  // binary on a Max/Pro plan — show which binary/plan alongside the model, since
  // the alias alone is identical on every machine.
  const runtimeBadge = resolveRuntimeBadge(entry)

  const isInherit = allowInherit && provider === ''

  const selectProvider = (p: string) => {
    const e = catalog.find((c) => c.id === p)
    onChange(p, e?.models[0]?.id ?? '')
    setCustom(false)
  }

  return (
    <div className="grid grid-cols-2 gap-3">
      <label className="block space-y-1">
        <span className="text-xs font-medium text-[var(--color-text-dim)]">Sağlayıcı</span>
        <select
          data-testid="provider-select"
          value={provider}
          onChange={(e) => selectProvider(e.target.value)}
          className={inputCls}
        >
          {allowInherit && <option value="">{inheritLabel}</option>}
          {!entry && provider !== '' && <option value={provider}>{provider}</option>}
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
        {isInherit ? (
          <input
            data-testid="model-custom-input"
            value={model}
            onChange={(e) => onChange('', e.target.value)}
            placeholder={inheritLabel}
            className={inputCls}
          />
        ) : showCustom ? (
          <div className="flex gap-1">
            <input
              data-testid="model-custom-input"
              value={model}
              onChange={(e) => onChange(provider, e.target.value)}
              placeholder="model adı"
              className={inputCls}
            />
            {entry && (
              <button
                type="button"
                data-testid="model-reset-to-list"
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
            data-testid="model-select"
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
            {models.map((m) => {
              const w = formatContextWindow(m.contextWindow)
              return (
                <option key={m.id || '__default__'} value={m.id}>
                  {m.label}
                  {w ? ` · ${w}` : ''}
                </option>
              )
            })}
            {(entry?.allowCustomModel ?? true) && <option value="__custom__">Özel…</option>}
          </select>
        )}
        {(runtimeBadge || (!showCustom && (selectedDesc || selectedWindow))) && (
          <span className="flex items-center gap-1.5 text-xs text-[var(--color-text-dim)]">
            {/* Shown for a custom model id too: the alias changes, the CLI behind
                it does not. */}
            {runtimeBadge && (
              <span
                data-testid="model-runtime-badge"
                title="Bu modeli çalıştıran yerel Claude Code kurulumu ve abonelik planı"
                className="shrink-0 rounded bg-[var(--color-surface-2)] px-1.5 py-px font-medium text-[var(--color-text)]"
              >
                {runtimeBadge}
              </span>
            )}
            {!showCustom && selectedWindow && (
              <span
                title="Yaklaşık bağlam penceresi (token)"
                className="shrink-0 rounded bg-[var(--color-surface-2)] px-1.5 py-px font-medium text-[var(--color-text)]"
              >
                {selectedWindow} bağlam
              </span>
            )}
            {!showCustom && selectedDesc && (
              <span className="min-w-0 truncate">{selectedDesc}</span>
            )}
          </span>
        )}
      </label>
    </div>
  )
}
