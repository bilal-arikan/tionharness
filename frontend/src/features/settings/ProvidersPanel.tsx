// Providers category: default provider/model picker, Anthropic + MiniMax +
// OpenRouter keys, custom (user-added) providers and connection test. (Anthropic
// beta toggles now live under the "Bağlam & Bellek" category.)
//
// Layout is deliberately table-like: the three built-in providers render as a
// uniform grid of cards (status badge + aligned key/endpoint columns + test) so
// the section reads as rows of the same shape, and custom providers render as a
// real table.
import { useEffect, useState } from 'react'
import { Sparkles, Zap, KeyRound, Boxes, Plus, Trash2, Network, type LucideIcon } from 'lucide-react'
import { api } from '@/api'
import type { AppSettings, ProviderTestResult, Secret } from '@/types'
import type { CustomProvider, UpsertProviderInput, PriceTable } from '@/api/providers'
import { inputCls, type AppSet } from './primitives'
import { ClaudeAuthDialog } from './ClaudeAuthDialog'

// fmtPrice formats a USD/1M-token figure compactly (e.g. "$0.30", "$15", "ücretsiz").
function fmtPrice(n: number): string {
  if (n === 0) return 'ücretsiz'
  return '$' + (n < 1 ? n.toFixed(2).replace(/0+$/, '').replace(/\.$/, '') : String(n))
}

// cacheModeLabel describes a prompt-cache mode for the capability badge.
function cacheModeLabel(mode?: string): string {
  switch (mode) {
    case 'native': return 'Cache: ✅ cache_control'
    case 'auto': return 'Cache: ✅ otomatik'
    case 'none': return 'Cache: ❌ yok'
    default: return 'Cache: ? bilinmiyor'
  }
}

function testBadge(test: Props['test'], provider: string) {
  const r = test[provider]
  if (!r) return null
  if (r === 'pending') return <span className="text-xs text-[var(--color-warning)]">test ediliyor…</span>
  if (r.ok) return <span className="text-xs text-[var(--color-success)]">✓ bağlandı{r.model ? ` (${r.model})` : ''}</span>
  return <span className="text-xs text-[var(--color-danger)]">✗ {r.error}</span>
}

interface Props {
  draft: AppSettings
  set: AppSet
  setDraft: React.Dispatch<React.SetStateAction<AppSettings | null>>
  test: Record<string, ProviderTestResult | 'pending'>
  runTest: (provider: string, model?: string) => void
  clearKey: (which: 'anthropic' | 'minimax' | 'openrouter') => void
  // Apply a provider key immediately (resolved from a vault secret). Provider
  // keys are never typed — only selected from the secret store.
  applyKey: (which: 'anthropic' | 'minimax' | 'openrouter', value: string) => void | Promise<void>
  // Secrets vault (this workspace), reveal a value to import as a key, and a
  // jump to the Secrets screen for managing them.
  secrets: Secret[]
  onImportSecret: (name: string) => Promise<string>
  onManageSecrets: () => void
  // Active workspace's resolved claude-cli config home (<workspace>/claude-home),
  // shown read-only in the claude config field. Empty falls back to the app-global
  // claudeConfigDir. This is what actually differs per workspace — the global draft
  // value is identical for all workspaces and was previously (wrongly) shown here.
  workspaceClaudeHome?: string
}

// KeyPicker is the vault-only key selector (no free text): the key can only be
// chosen from the secret vault (revealed + applied), reflecting the policy that
// provider secrets live in the vault, not in a plaintext settings field. A
// compact variant for the built-in provider cards.
function KeyPicker({
  isSet,
  secrets,
  onPick,
  onClear,
  provider,
}: {
  isSet: boolean
  secrets: Secret[]
  onPick: (name: string) => void
  onClear: () => void
  provider: string
}) {
  return (
    <div className="flex items-center gap-1.5">
      <select
        data-testid="provider-key-picker"
        data-provider={provider}
        defaultValue=""
        disabled={secrets.length === 0}
        onChange={(e) => {
          const name = e.target.value
          e.currentTarget.selectedIndex = 0
          if (name) onPick(name)
        }}
        className={`${inputCls} min-w-0 flex-1`}
      >
        <option value="">
          {secrets.length ? (isSet ? 'Değiştir: sırdan seç…' : 'Sırdan seç…') : 'Sır yok'}
        </option>
        {secrets.map((s) => (
          <option key={s.name} value={s.name}>{s.name}</option>
        ))}
      </select>
      {isSet && (
        <button
          data-testid="provider-clear-key"
          data-provider={provider}
          onClick={onClear}
          className="shrink-0 rounded border border-[var(--color-border)] px-2 py-1.5 text-xs text-[var(--color-danger)] hover:border-[var(--color-danger)]"
        >
          Sil
        </button>
      )}
    </div>
  )
}

// BuiltinProvider is one row of the built-in providers table, rendered as a card
// with a uniform shape: header (icon + name + kind + status badge), an aligned
// two-column body (key picker | endpoint), and a footer test row. The endpoint
// field is generic so Anthropic (claude CLI path) and the OpenAI-compatible
// providers (base URL) share the exact same layout.
function BuiltinProvider({
  icon: Icon,
  name,
  kindLabel,
  keyLabel,
  isSet,
  requiredHint,
  secrets,
  onPick,
  onClear,
  endpointLabel,
  endpointValue,
  endpointPlaceholder,
  onEndpoint,
  hideEndpoint,
  endpoint2Label,
  endpoint2Value,
  endpoint2Placeholder,
  onEndpoint2,
  endpoint2Hint,
  endpoint2ReadOnly,
  extra,
  test,
  runTest,
  testProvider,
  testModel,
  testDisabledHint,
}: {
  icon: LucideIcon
  name: string
  kindLabel: string
  keyLabel: string
  isSet: boolean
  requiredHint: string
  secrets: Secret[]
  onPick: (name: string) => void
  onClear: () => void
  endpointLabel: string
  endpointValue: string
  endpointPlaceholder: string
  onEndpoint: (v: string) => void
  // When true, the endpoint column is not rendered and the key picker spans the
  // full row (used for cards whose auth has no endpoint concept, e.g. a pure API-key card).
  hideEndpoint?: boolean
  // Optional second endpoint-style field (e.g. claude CLI config dir). Rendered
  // full-width below the key/endpoint grid only when all four props are supplied.
  endpoint2Label?: string
  endpoint2Value?: string
  endpoint2Placeholder?: string
  onEndpoint2?: (v: string) => void
  endpoint2Hint?: string
  // When true the second field is display-only (used for claudeConfigDir, which is
  // now a per-workspace-derived value and only a fallback — not user-editable).
  endpoint2ReadOnly?: boolean
  // Optional extra controls (e.g. a "kimlik doğrula" button) rendered full-width
  // between the endpoint fields and the test footer.
  extra?: React.ReactNode
  test: Props['test']
  runTest: Props['runTest']
  testProvider: string
  testModel?: string
  testDisabledHint: string
}) {
  return (
    <div className="space-y-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
      <div className="flex items-center justify-between gap-2">
        <div className="flex min-w-0 items-center gap-2">
          <Icon size={15} className="shrink-0 text-[var(--color-accent)]" />
          <span className="truncate text-sm font-medium">{name}</span>
          <span className="shrink-0 text-[10px] uppercase tracking-wide text-[var(--color-text-dim)]">{kindLabel}</span>
        </div>
        <span
          className={`shrink-0 rounded px-1.5 py-0.5 text-[10px] font-medium ${
            isSet ? 'bg-[var(--color-surface-2)] text-[var(--color-success)]' : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)]'
          }`}
        >
          {isSet ? '✓ Anahtar kayıtlı' : 'Anahtar yok'}
        </span>
      </div>

      <div className={hideEndpoint ? 'grid gap-2' : 'grid gap-2 sm:grid-cols-2'}>
        <div className="flex flex-col gap-1">
          <span className="text-xs font-medium text-[var(--color-text-dim)]">{keyLabel}</span>
          <KeyPicker isSet={isSet} secrets={secrets} onPick={onPick} onClear={onClear} provider={testProvider} />
        </div>
        {!hideEndpoint && (
          <div className="flex flex-col gap-1">
            <span className="text-xs font-medium text-[var(--color-text-dim)]">{endpointLabel}</span>
            <input
              data-testid="provider-endpoint-input"
              data-provider={testProvider}
              value={endpointValue}
              onChange={(e) => onEndpoint(e.target.value)}
              placeholder={endpointPlaceholder}
              className={inputCls}
            />
          </div>
        )}
      </div>

      {endpoint2Label && onEndpoint2 && (
        <div className="flex flex-col gap-1">
          <span className="text-xs font-medium text-[var(--color-text-dim)]">{endpoint2Label}</span>
          <input
            value={endpoint2Value ?? ''}
            onChange={(e) => { if (!endpoint2ReadOnly) onEndpoint2(e.target.value) }}
            readOnly={endpoint2ReadOnly}
            placeholder={endpoint2Placeholder}
            className={endpoint2ReadOnly ? `${inputCls} cursor-not-allowed opacity-60` : inputCls}
          />
          {endpoint2Hint && <span className="text-[10px] text-[var(--color-text-dim)]">{endpoint2Hint}</span>}
        </div>
      )}

      {extra}

      <div className="flex items-center justify-between gap-2">
        <span className="min-w-0 truncate text-[11px] text-[var(--color-text-dim)]">
          {isSet ? '✓ Kayıtlı (şifreli).' : requiredHint}
        </span>
        <div className="flex shrink-0 items-center gap-2">
          {!isSet
            ? <span className="text-xs text-[var(--color-text-dim)]">{testDisabledHint}</span>
            : testBadge(test, testProvider)}
          <button
            data-testid="provider-test"
            data-provider={testProvider}
            onClick={() => runTest(testProvider, testModel)}
            disabled={!isSet}
            className="rounded border border-[var(--color-border)] px-2 py-1.5 text-xs hover:border-[var(--color-accent)] disabled:opacity-40 disabled:hover:border-[var(--color-border)]"
          >
            Test et
          </button>
        </div>
      </div>
    </div>
  )
}

// SecretSource lets the user fill the custom-provider key field from the
// workspace secret vault (revealed and imported into the field, then saved
// encrypted) and jump to the Secrets screen to manage entries.
function SecretSource({
  secrets,
  onPick,
  onManage,
}: {
  secrets: Secret[]
  onPick: (name: string) => void
  onManage: () => void
}) {
  return (
    <div className="mt-1 flex items-center gap-2">
      <select
        defaultValue=""
        onChange={(e) => {
          const name = e.target.value
          e.currentTarget.selectedIndex = 0
          if (name) onPick(name)
        }}
        className={`${inputCls} flex-1 text-xs`}
        disabled={secrets.length === 0}
      >
        <option value="">
          {secrets.length ? '🔑 Sırlardan içe aktar…' : 'Sır yok'}
        </option>
        {secrets.map((s) => (
          <option key={s.name} value={s.name}>{s.name}</option>
        ))}
      </select>
      <button
        onClick={onManage}
        className="flex items-center gap-1 rounded border border-[var(--color-border)] px-2 py-1.5 text-xs hover:border-[var(--color-accent)]"
      >
        <KeyRound size={12} /> Sırları yönet →
      </button>
    </div>
  )
}

const EMPTY_PROVIDER: UpsertProviderInput = {
  id: '', label: '', kind: 'openai', baseUrl: '', defaultModel: '', models: '', key: '',
  reasoning: false, promptCache: '',
}

// CustomProviders manages user-added OpenAI/Anthropic-compatible endpoints
// (OpenRouter, Gemini, Kimi, Ollama, ...). It fetches and mutates the list via
// the dedicated /api/providers endpoints, independent of the main settings save.
// The list renders as a real table; the add/edit form sits below it.
function CustomProviders({
  secrets,
  onImportSecret,
  onManageSecrets,
}: {
  secrets: Secret[]
  onImportSecret: (name: string) => Promise<string>
  onManageSecrets: () => void
}) {
  const [list, setList] = useState<CustomProvider[]>([])
  const [draft, setDraft] = useState<UpsertProviderInput>(EMPTY_PROVIDER)
  const [editing, setEditing] = useState(false)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  // Ballpark list prices (provider id → model → price) for the per-provider cost hint.
  const [prices, setPrices] = useState<PriceTable>({})

  useEffect(() => {
    api.listCustomProviders().then(setList).catch(() => {})
    api.prices().then(setPrices).catch(() => {})
  }, [])

  const reset = () => { setDraft(EMPTY_PROVIDER); setEditing(false); setErr('') }
  const upd = (patch: Partial<UpsertProviderInput>) => setDraft((d) => ({ ...d, ...patch }))

  const save = async () => {
    setBusy(true); setErr('')
    try {
      const payload: UpsertProviderInput = { ...draft }
      // Blank key on save = keep the stored one (backend treats omitted as keep).
      if (!payload.key) delete payload.key
      setList(await api.upsertCustomProvider(payload))
      reset()
    } catch (e) {
      setErr((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  const edit = (p: CustomProvider) => {
    setDraft({ id: p.id, label: p.label, kind: p.kind, baseUrl: p.baseUrl, defaultModel: p.defaultModel, models: p.models, key: '', reasoning: !!p.reasoning, promptCache: p.promptCache ?? '' })
    setEditing(true); setErr('')
  }
  const remove = async (id: string) => {
    try {
      setList(await api.deleteCustomProvider(id))
      if (draft.id === id) reset()
    } catch (e) {
      setErr((e as Error).message)
    }
  }

  return (
    <div className="space-y-2">
      {list.length > 0 && (
        <div className="grid gap-2">
          {list.map((p) => (
            <div key={p.id} className="space-y-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
              <div className="flex items-center justify-between gap-2">
                <div className="flex min-w-0 items-center gap-2">
                  <Boxes size={15} className="shrink-0 text-[var(--color-accent)]" />
                  <span className="truncate text-sm font-medium">{p.label || p.id}</span>
                  <span className="shrink-0 text-[10px] uppercase tracking-wide text-[var(--color-text-dim)]">
                    {p.kind === 'anthropic' ? 'Anthropic-uyumlu' : 'OpenAI-uyumlu'}
                  </span>
                </div>
                <span
                  className={`shrink-0 rounded px-1.5 py-0.5 text-[10px] font-medium ${
                    p.keySet ? 'bg-[var(--color-surface-2)] text-[var(--color-success)]' : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)]'
                  }`}
                >
                  {p.keySet ? '✓ Anahtar kayıtlı' : 'Anahtar yok'}
                </span>
              </div>

              <div className="grid gap-x-3 gap-y-1 text-xs sm:grid-cols-2">
                <div className="min-w-0 truncate" title={p.id}>
                  <span className="text-[var(--color-text-dim)]">id: </span>{p.id}
                </div>
                <div className="min-w-0 truncate" title={p.defaultModel}>
                  <span className="text-[var(--color-text-dim)]">model: </span>{p.defaultModel || '—'}
                  {(() => {
                    const pr = prices[p.id]?.[p.defaultModel]
                    return pr ? (
                      <span className="ml-1 tabular-nums text-[var(--color-text-dim)]" title="giriş / çıkış — $/1M token">
                        ({fmtPrice(pr.inputPerMTok)} / {fmtPrice(pr.outputPerMTok)})
                      </span>
                    ) : null
                  })()}
                </div>
                <div className="col-span-full min-w-0 truncate text-[var(--color-text-dim)]" title={p.baseUrl}>{p.baseUrl}</div>
              </div>

              <div className="flex flex-wrap gap-1.5">
                <span className="rounded px-1.5 py-0.5 text-[10px]" style={{ background: p.reasoning ? 'var(--color-success, #16a34a)22' : 'var(--color-surface-2)', color: p.reasoning ? 'var(--color-success, #16a34a)' : 'var(--color-text-dim)' }}>
                  {p.reasoning ? 'Düşünme: ✅' : 'Düşünme: —'}
                </span>
                <span className="rounded px-1.5 py-0.5 text-[10px]" style={{ background: (p.promptCache === 'native' || p.promptCache === 'auto') ? 'var(--color-success, #16a34a)22' : 'var(--color-surface-2)', color: (p.promptCache === 'native' || p.promptCache === 'auto') ? 'var(--color-success, #16a34a)' : 'var(--color-text-dim)' }}>
                  {cacheModeLabel(p.promptCache)}
                </span>
              </div>

              <div className="flex items-center justify-end gap-1.5">
                <button data-testid="custom-provider-edit" data-provider-id={p.id} onClick={() => edit(p)} className="rounded border border-[var(--color-border)] px-2 py-1 text-xs hover:border-[var(--color-accent)]">Düzenle</button>
                <button data-testid="custom-provider-delete" data-provider-id={p.id} onClick={() => remove(p.id)} className="rounded border border-[var(--color-border)] p-1 text-[var(--color-danger)] hover:border-[var(--color-danger)]"><Trash2 size={13} /></button>
              </div>
            </div>
          ))}
        </div>
      )}

      <div className="space-y-1.5 rounded-md border border-dashed border-[var(--color-border)] p-2">
        <div className="text-xs font-medium">{editing ? `Düzenle: ${draft.id}` : 'Yeni özel sağlayıcı'}</div>
        <div className="grid grid-cols-2 gap-1.5">
          <input data-testid="custom-provider-id-input" placeholder="id (ör. openrouter)" value={draft.id} disabled={editing} onChange={(e) => upd({ id: e.target.value })} className={inputCls} />
          <input data-testid="custom-provider-label-input" placeholder="Etiket" value={draft.label} onChange={(e) => upd({ label: e.target.value })} className={inputCls} />
          <select data-testid="custom-provider-kind-select" value={draft.kind} onChange={(e) => upd({ kind: e.target.value })} className={inputCls}>
            <option value="openai">OpenAI-uyumlu (tool-use)</option>
            <option value="anthropic">Anthropic-uyumlu (tool-use + thinking)</option>
          </select>
          <input data-testid="custom-provider-default-model-input" placeholder="varsayılan model" value={draft.defaultModel} onChange={(e) => upd({ defaultModel: e.target.value })} className={inputCls} />
        </div>
        <input data-testid="custom-provider-base-url-input" placeholder="base URL (ör. https://openrouter.ai/api/v1)" value={draft.baseUrl} onChange={(e) => upd({ baseUrl: e.target.value })} className={inputCls} />
        <input data-testid="custom-provider-models-input" placeholder="model id'leri — virgülle, opsiyonel" value={draft.models} onChange={(e) => upd({ models: e.target.value })} className={inputCls} />

        {/* Capability flags: reasoning passthrough + prompt-cache mode. */}
        <div className="grid grid-cols-2 items-center gap-1.5">
          <label className="flex items-center gap-1.5 text-xs" title="Açıksa OpenAI-uyumlu uca reasoning_effort gönderilir (ajanın Düşünme seviyesinden). Anthropic-uyumlu uçlar thinking'i native destekler.">
            <input data-testid="custom-provider-reasoning" type="checkbox" checked={!!draft.reasoning} onChange={(e) => upd({ reasoning: e.target.checked })} />
            Düşünme (reasoning_effort)
          </label>
          <select data-testid="custom-provider-cache-select" value={draft.promptCache || ''} onChange={(e) => upd({ promptCache: e.target.value })} className={inputCls} title="Prompt-cache davranışı: native = cache_control enjekte; auto = sunucu otomatik; none = yok.">
            <option value="">Cache: bilinmiyor</option>
            <option value="native">Cache: native (cache_control)</option>
            <option value="auto">Cache: otomatik</option>
            <option value="none">Cache: yok</option>
          </select>
        </div>

        {/* Per-model price view (read-only) — shown when prices are known for this id. */}
        {editing && (() => {
          const table = prices[draft.id]
          const ids = draft.models.split(/[\n,]/).map((s) => s.trim()).filter(Boolean)
          if (!table || ids.length === 0) return null
          const priced = ids.filter((m) => table[m])
          if (priced.length === 0) return null
          return (
            <div className="rounded bg-[var(--color-surface-2)] p-1.5">
              <div className="mb-1 text-[10px] uppercase tracking-wide text-[var(--color-text-dim)]">Fiyatlar — $/1M token (giriş / çıkış)</div>
              <div className="max-h-40 overflow-y-auto">
                {priced.map((m) => {
                  const pr = table[m]
                  return (
                    <div key={m} className="flex items-center justify-between gap-3 px-1 py-0.5 text-[11px]">
                      <span className="min-w-0 break-all font-mono">{m}</span>
                      <span className="shrink-0 tabular-nums text-[var(--color-text-dim)]">{fmtPrice(pr.inputPerMTok)} / {fmtPrice(pr.outputPerMTok)}</span>
                    </div>
                  )
                })}
              </div>
              <p className="mt-1 text-[10px] text-[var(--color-text-dim)]">Yaklaşık liste fiyatı; bütçe ekranı bu değerlerle maliyet hesaplar.</p>
            </div>
          )
        })()}

        <SecretSource secrets={secrets} onManage={onManageSecrets} onPick={async (n) => upd({ key: await onImportSecret(n) })} />
        <div className="text-xs text-[var(--color-text-dim)]">
          {draft.key ? '✓ Anahtar sırdan seçildi' : editing ? 'Anahtar korunacak (değiştirmek için sırdan seç)' : 'Anahtar: yalnızca sırdan seçilir (elle giriş kapalı)'}
        </div>
        {err && <div className="text-xs text-[var(--color-danger)]">{err}</div>}
        <div className="flex gap-2">
          <button data-testid="custom-provider-save" onClick={save} disabled={busy || !draft.id || !draft.baseUrl} className="flex items-center gap-1 rounded bg-[var(--color-accent)] px-3 py-1.5 text-xs font-medium text-white hover:opacity-90 disabled:opacity-30">
            <Plus size={13} /> {editing ? 'Güncelle' : 'Ekle'}
          </button>
          {editing && <button data-testid="custom-provider-cancel" onClick={reset} className="rounded border border-[var(--color-border)] px-3 py-1.5 text-xs">İptal</button>}
        </div>
      </div>
    </div>
  )
}

export function ProvidersPanel({
  draft,
  set,
  setDraft,
  test,
  runTest,
  clearKey,
  applyKey,
  secrets,
  onImportSecret,
  onManageSecrets,
  workspaceClaudeHome,
}: Props) {
  const [authOpen, setAuthOpen] = useState(false)
  // Pre-flight login check for THIS workspace's claude-home (distinct from the
  // generic "test et", which probes the app-global config dir). 'idle' before run.
  const [wsAuth, setWsAuth] = useState<
    'idle' | 'pending' | { loggedIn: boolean; detail?: string }
  >('idle')
  const checkWsAuth = async () => {
    setWsAuth('pending')
    try {
      const r = await api.checkWorkspaceClaudeAuth()
      setWsAuth({ loggedIn: r.loggedIn, detail: r.detail })
    } catch (e) {
      setWsAuth({ loggedIn: false, detail: (e as Error).message })
    }
  }
  return (
    <>
      {authOpen && (
        <ClaudeAuthDialog
          configDir={draft.claudeConfigDir}
          currentKind={draft.claudeCliAuthKind}
          isSet={draft.claudeCliAuthSet}
          onClose={() => setAuthOpen(false)}
          onSaved={(next) => setDraft(next)}
        />
      )}
      <div>
        <div className="mb-1.5 flex items-center justify-between gap-2">
          <span className="text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">Yerleşik sağlayıcılar</span>
          <button
            onClick={onManageSecrets}
            className="flex items-center gap-1 text-xs text-[var(--color-text-dim)] hover:text-[var(--color-accent)]"
          >
            <KeyRound size={12} /> Sırları yönet →
          </button>
        </div>
        <p className="mb-2 text-xs text-[var(--color-text-dim)]">Anahtarlar yalnızca Sır kasasından seçilir — elle giriş kapalı.</p>
        <div className="grid gap-2">
          <div className="space-y-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
            <div className="flex items-center justify-between gap-2">
              <div className="flex min-w-0 items-center gap-2">
                <Sparkles size={15} className="shrink-0 text-[var(--color-accent)]" />
                <span className="truncate text-sm font-medium">Anthropic Pro/Max (OAuth)</span>
                <span className="shrink-0 text-[10px] uppercase tracking-wide text-[var(--color-text-dim)]">claude-cli / abonelik</span>
              </div>
              <span
                className={`shrink-0 rounded px-1.5 py-0.5 text-[10px] font-medium ${
                  draft.claudeCliAuthSet ? 'bg-[var(--color-surface-2)] text-[var(--color-success)]' : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)]'
                }`}
              >
                {draft.claudeCliAuthSet
                  ? `✓ ${draft.claudeCliAuthKind === 'apikey' ? 'API token' : 'Max/Pro token'} kayıtlı`
                  : 'Kimlik yok'}
              </span>
            </div>

            <div className="flex flex-col gap-1">
              <span className="text-xs font-medium text-[var(--color-text-dim)]">claude CLI yolu</span>
              <input
                data-testid="provider-endpoint-input"
                data-provider="claude-cli"
                value={draft.claudeCliPath}
                onChange={(e) => set('claudeCliPath', e.target.value)}
                placeholder="otomatik (PATH)"
                className={inputCls}
              />
            </div>

            <div className="flex flex-col gap-1">
              <span className="text-xs font-medium text-[var(--color-text-dim)]">claude config dizini (bu workspace · salt-okunur)</span>
              <input
                value={workspaceClaudeHome || draft.claudeConfigDir}
                readOnly
                placeholder="per-workspace: <workspace>/claude-home"
                className={`${inputCls} cursor-not-allowed opacity-60`}
              />
              <span className="text-[10px] text-[var(--color-text-dim)]">
                {workspaceClaudeHome
                  ? 'Aktif workspace\'in kendi CLAUDE_CONFIG_DIR yolu — skill/ayar/login bu workspace ile paylaşılır. Her workspace farklı bir yol kullanır; salt-okunur (workspace kökünden türetilir).'
                  : 'Uygulama-geneli fallback (workspace çözülemedi). Normalde her workspace kendi <workspace>/claude-home dizinini kullanır; salt-okunur.'}
              </span>
            </div>

            <div className="space-y-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] p-2.5">
              <div className="flex flex-wrap items-center gap-2">
                <button
                  data-testid="claude-auth-open"
                  onClick={() => setAuthOpen(true)}
                  className="flex items-center gap-1.5 rounded-lg border border-[var(--color-border)] px-2.5 py-1.5 text-xs hover:border-[var(--color-accent)]"
                >
                  <KeyRound size={12} /> claude-cli kimlik (Max / API)
                </button>
                <span className="text-[11px] text-[var(--color-text-dim)]">
                  {draft.claudeCliAuthSet
                    ? `✓ ${draft.claudeCliAuthKind === 'oauth' ? 'Max/Pro token' : 'API anahtarı'} kayıtlı`
                    : 'İzole dizin için token ekle (login gerekmez)'}
                </span>
              </div>
              {/* Dedicated claude-cli probe: runs the `claude` binary with the
                  config dir + injected token, separate from the Anthropic HTTP
                  API-key test (which would fail with "invalid x-api-key" when only
                  a Max/Pro OAuth token is set). */}
              <div className="flex flex-wrap items-center gap-2">
                <button
                  data-testid="provider-test"
                  data-provider="claude-cli"
                  onClick={() => runTest('claude-cli')}
                  className="flex items-center gap-1.5 rounded-lg border border-[var(--color-border)] px-2.5 py-1.5 text-xs hover:border-[var(--color-accent)]"
                >
                  <Sparkles size={12} /> claude-cli'yi test et
                </button>
                {testBadge(test, 'claude-cli')}
              </div>
              {/* Pre-flight: verify THIS workspace's claude-home is logged in
                  before an agent turn burns on an auth wall. Cheap tool-free probe
                  against <workspace>/claude-home (not the app-global config dir). */}
              <div className="flex flex-wrap items-center gap-2">
                <button
                  data-testid="workspace-claude-auth-check"
                  onClick={checkWsAuth}
                  className="flex items-center gap-1.5 rounded-lg border border-[var(--color-border)] px-2.5 py-1.5 text-xs hover:border-[var(--color-accent)]"
                >
                  <KeyRound size={12} /> Bu workspace login doğrula
                </button>
                {wsAuth === 'pending' && (
                  <span className="text-[11px] text-[var(--color-warning)]">kontrol ediliyor…</span>
                )}
                {typeof wsAuth === 'object' && wsAuth.loggedIn && (
                  <span className="text-[11px] text-[var(--color-success)]">✓ giriş yapılmış</span>
                )}
                {typeof wsAuth === 'object' && !wsAuth.loggedIn && (
                  <span className="text-[11px] text-[var(--color-error)]" title={wsAuth.detail}>
                    ✕ giriş yok — {wsAuth.detail || 'claude /login gerekli'}
                  </span>
                )}
              </div>
            </div>
          </div>
          <BuiltinProvider
            icon={Sparkles}
            name="Anthropic API"
            kindLabel="API"
            keyLabel="API anahtarı"
            isSet={draft.anthropicKeySet}
            requiredHint="anthropic (HTTP API) sağlayıcısı için gerekli."
            secrets={secrets}
            onPick={async (n) => applyKey('anthropic', await onImportSecret(n))}
            onClear={() => clearKey('anthropic')}
            hideEndpoint
            endpointLabel=""
            endpointValue=""
            endpointPlaceholder=""
            onEndpoint={() => {}}
            test={test}
            runTest={runTest}
            testProvider="anthropic"
            testDisabledHint="önce Anthropic API anahtarı ekle"
          />
          <BuiltinProvider
            icon={Zap}
            name="MiniMax"
            kindLabel="OpenAI-uyumlu"
            keyLabel="API anahtarı"
            isSet={draft.minimaxKeySet}
            requiredHint="MiniMax modelleri için gerekli."
            secrets={secrets}
            onPick={async (n) => applyKey('minimax', await onImportSecret(n))}
            onClear={() => clearKey('minimax')}
            endpointLabel="base URL"
            endpointValue={draft.minimaxBaseUrl}
            endpointPlaceholder="https://api.minimax.io/v1"
            onEndpoint={(v) => set('minimaxBaseUrl', v)}
            test={test}
            runTest={runTest}
            testProvider="minimax"
            testModel="MiniMax-M3"
            testDisabledHint="önce MiniMax anahtarı ekle"
          />
          <BuiltinProvider
            icon={Network}
            name="OpenRouter"
            kindLabel="OpenAI-uyumlu"
            keyLabel="API anahtarı"
            isSet={draft.openrouterKeySet}
            requiredHint="OpenRouter modelleri için gerekli (tek anahtar, yüzlerce model)."
            secrets={secrets}
            onPick={async (n) => applyKey('openrouter', await onImportSecret(n))}
            onClear={() => clearKey('openrouter')}
            endpointLabel="base URL"
            endpointValue={draft.openrouterBaseUrl}
            endpointPlaceholder="https://openrouter.ai/api/v1"
            onEndpoint={(v) => set('openrouterBaseUrl', v)}
            test={test}
            runTest={runTest}
            testProvider="openrouter"
            testModel="anthropic/claude-sonnet-4.6"
            testDisabledHint="önce OpenRouter anahtarı ekle"
          />
        </div>
      </div>

      <div className="flex items-center gap-1.5 pt-2 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
        <Boxes size={13} className="text-[var(--color-accent)]" /> Özel sağlayıcılar
      </div>
      <p className="-mt-1 text-xs text-[var(--color-text-dim)]">OpenAI- veya Anthropic-uyumlu herhangi bir uç (OpenRouter, Gemini, Kimi, Ollama…). Eklenince ajan oluştururken sağlayıcı olarak seçilebilir. Değişiklikler anında kaydedilir (üstteki Kaydet'ten bağımsız).</p>
      <CustomProviders secrets={secrets} onImportSecret={onImportSecret} onManageSecrets={onManageSecrets} />
    </>
  )
}
