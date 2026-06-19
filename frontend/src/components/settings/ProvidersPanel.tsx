// Providers category: default provider/model picker, Anthropic + MiniMax +
// OpenRouter keys, custom (user-added) providers and connection test. (Anthropic
// beta toggles now live under the "Bağlam & Bellek" category.)
import { useEffect, useState } from 'react'
import { Server, Sparkles, Zap, KeyRound, Boxes, Plus, Trash2, Network } from 'lucide-react'
import { api } from '../../api'
import type { AppSettings, ProviderTestResult, Secret } from '../../types'
import type { CustomProvider, UpsertProviderInput } from '../../api/providers'
import { ProviderModelSelect } from '../agents/ProviderModelSelect'
import { Field, inputCls, type AppSet } from './primitives'

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
}

function testBadge(test: Props['test'], provider: string) {
  const r = test[provider]
  if (!r) return null
  if (r === 'pending') return <span className="text-xs text-[var(--color-warning)]">test ediliyor…</span>
  if (r.ok) return <span className="text-xs text-[var(--color-success)]">✓ bağlandı{r.model ? ` (${r.model})` : ''}</span>
  return <span className="text-xs text-[var(--color-danger)]">✗ {r.error}</span>
}

// TestConnection is a reusable "test the connection" row for a provider section.
// It probes the given provider id (optionally with a representative model, since
// the global default model usually belongs to a different provider) and shows
// the live result badge. Disabled until the provider has a key configured.
function TestConnection({
  test,
  runTest,
  provider,
  model,
  disabled,
  disabledHint,
}: {
  test: Props['test']
  runTest: Props['runTest']
  provider: string
  model?: string
  disabled?: boolean
  disabledHint?: string
}) {
  return (
    <div className="mt-1 flex items-center gap-2">
      <button
        onClick={() => runTest(provider, model)}
        disabled={disabled}
        className="rounded border border-[var(--color-border)] px-2 py-1.5 text-xs hover:border-[var(--color-accent)] disabled:opacity-40 disabled:hover:border-[var(--color-border)]"
      >
        Bağlantıyı test et
      </button>
      {disabled
        ? <span className="text-xs text-[var(--color-text-dim)]">{disabledHint ?? 'önce anahtar ekle'}</span>
        : testBadge(test, provider)}
    </div>
  )
}

// SecretSource lets the user fill a key field from the workspace secret vault
// (revealed and imported into the field, then saved encrypted into settings)
// and jump to the Secrets screen to manage entries.
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

// ProviderKeyField is a key control with NO free text entry: the key can only
// be chosen from the secret vault (revealed + applied), reflecting the policy
// that provider secrets live in the vault, not in a plaintext settings field.
function ProviderKeyField({
  label,
  isSet,
  requiredHint,
  secrets,
  onPick,
  onClear,
  onManage,
}: {
  label: string
  isSet: boolean
  requiredHint: string
  secrets: Secret[]
  onPick: (name: string) => void
  onClear: () => void
  onManage: () => void
}) {
  return (
    <Field label={label} hint="Anahtar yalnızca Sır kasasından seçilir — elle giriş kapalı.">
      <div className="flex items-center gap-2">
        <select
          defaultValue=""
          disabled={secrets.length === 0}
          onChange={(e) => {
            const name = e.target.value
            e.currentTarget.selectedIndex = 0
            if (name) onPick(name)
          }}
          className={`${inputCls} flex-1`}
        >
          <option value="">
            {secrets.length ? (isSet ? '🔑 Kayıtlı — değiştirmek için sırdan seç…' : 'Sırdan seç…') : 'Sır yok — önce ekle'}
          </option>
          {secrets.map((s) => (
            <option key={s.name} value={s.name}>{s.name}</option>
          ))}
        </select>
        {isSet && (
          <button onClick={onClear} className="rounded border border-[var(--color-border)] px-2 py-1.5 text-xs text-[var(--color-danger)] hover:border-[var(--color-danger)]">Sil</button>
        )}
        <button onClick={onManage} className="flex items-center gap-1 rounded border border-[var(--color-border)] px-2 py-1.5 text-xs hover:border-[var(--color-accent)]">
          <KeyRound size={12} /> Sırlar →
        </button>
      </div>
      <div className="mt-1 text-xs text-[var(--color-text-dim)]">{isSet ? '✓ Kayıtlı (şifreli).' : requiredHint}</div>
    </Field>
  )
}

const EMPTY_PROVIDER: UpsertProviderInput = {
  id: '', label: '', kind: 'openai', baseUrl: '', defaultModel: '', models: '', key: '',
}

// CustomProviders manages user-added OpenAI/Anthropic-compatible endpoints
// (OpenRouter, Gemini, Kimi, Ollama, ...). It fetches and mutates the list via
// the dedicated /api/providers endpoints, independent of the main settings save.
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

  useEffect(() => {
    api.listCustomProviders().then(setList).catch(() => {})
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
    setDraft({ id: p.id, label: p.label, kind: p.kind, baseUrl: p.baseUrl, defaultModel: p.defaultModel, models: p.models, key: '' })
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
      {list.map((p) => (
        <div key={p.id} className="flex items-center gap-2 rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] p-2 text-xs">
          <div className="min-w-0 flex-1">
            <div className="font-medium">
              {p.label} <span className="font-normal text-[var(--color-text-dim)]">· {p.id} · {p.kind === 'anthropic' ? 'Anthropic-uyumlu' : 'OpenAI-uyumlu'}</span>
            </div>
            <div className="truncate text-[var(--color-text-dim)]">{p.baseUrl}{p.defaultModel ? ` · ${p.defaultModel}` : ''}</div>
          </div>
          <span className={p.keySet ? 'text-[var(--color-success)]' : 'text-[var(--color-warning)]'}>{p.keySet ? '🔑' : 'anahtar yok'}</span>
          <button onClick={() => edit(p)} className="rounded border border-[var(--color-border)] px-2 py-1 hover:border-[var(--color-accent)]">Düzenle</button>
          <button onClick={() => remove(p.id)} className="rounded border border-[var(--color-border)] p-1 text-[var(--color-danger)] hover:border-[var(--color-danger)]"><Trash2 size={13} /></button>
        </div>
      ))}

      <div className="space-y-1.5 rounded-md border border-dashed border-[var(--color-border)] p-2">
        <div className="text-xs font-medium">{editing ? `Düzenle: ${draft.id}` : 'Yeni özel sağlayıcı'}</div>
        <div className="grid grid-cols-2 gap-1.5">
          <input placeholder="id (ör. openrouter)" value={draft.id} disabled={editing} onChange={(e) => upd({ id: e.target.value })} className={inputCls} />
          <input placeholder="Etiket" value={draft.label} onChange={(e) => upd({ label: e.target.value })} className={inputCls} />
          <select value={draft.kind} onChange={(e) => upd({ kind: e.target.value })} className={inputCls}>
            <option value="openai">OpenAI-uyumlu (tool-use)</option>
            <option value="anthropic">Anthropic-uyumlu (tool-use + thinking)</option>
          </select>
          <input placeholder="varsayılan model" value={draft.defaultModel} onChange={(e) => upd({ defaultModel: e.target.value })} className={inputCls} />
        </div>
        <input placeholder="base URL (ör. https://openrouter.ai/api/v1)" value={draft.baseUrl} onChange={(e) => upd({ baseUrl: e.target.value })} className={inputCls} />
        <input placeholder="model id'leri — virgülle, opsiyonel" value={draft.models} onChange={(e) => upd({ models: e.target.value })} className={inputCls} />
        <SecretSource secrets={secrets} onManage={onManageSecrets} onPick={async (n) => upd({ key: await onImportSecret(n) })} />
        <div className="text-xs text-[var(--color-text-dim)]">
          {draft.key ? '✓ Anahtar sırdan seçildi' : editing ? 'Anahtar korunacak (değiştirmek için sırdan seç)' : 'Anahtar: yalnızca sırdan seçilir (elle giriş kapalı)'}
        </div>
        {err && <div className="text-xs text-[var(--color-danger)]">{err}</div>}
        <div className="flex gap-2">
          <button onClick={save} disabled={busy || !draft.id || !draft.baseUrl} className="flex items-center gap-1 rounded bg-[var(--color-accent)] px-3 py-1.5 text-xs font-medium text-white hover:opacity-90 disabled:opacity-30">
            <Plus size={13} /> {editing ? 'Güncelle' : 'Ekle'}
          </button>
          {editing && <button onClick={reset} className="rounded border border-[var(--color-border)] px-3 py-1.5 text-xs">İptal</button>}
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
}: Props) {
  return (
    <>
      <div>
        <div className="mb-1 flex items-center gap-1.5 text-sm font-medium">
          <Server size={14} className="text-[var(--color-accent)]" />
          Varsayılan sağlayıcı + model
        </div>
        <p className="mb-2 text-xs text-[var(--color-text-dim)]">Yeni ajanlar bunlarla oluşturulur (model boş = sağlayıcı varsayılanı).</p>
        <ProviderModelSelect
          provider={draft.defaultProvider}
          model={draft.defaultModel}
          onChange={(p, m) => setDraft((d) => (d ? { ...d, defaultProvider: p, defaultModel: m } : d))}
        />
        <div className="mt-2 flex items-center gap-2">
          <button onClick={() => runTest(draft.defaultProvider)} className="rounded border border-[var(--color-border)] px-2 py-1.5 text-xs hover:border-[var(--color-accent)]">
            Bağlantıyı test et
          </button>
          {testBadge(test, draft.defaultProvider)}
        </div>
      </div>

      <Field label="Yeni ajan varsayılan izin modu" hint="Yeni oluşturulan ajanların araç-kullanım izni. Mevcut ajanları Ajanlar ekranından, tek tur için Composer'dan (Shift+Tab) değiştir.">
        <select value={draft.defaultPermissionMode || 'auto'} onChange={(e) => set('defaultPermissionMode', e.target.value)} className={inputCls}>
          <option value="auto">Otomatik — tüm araçlar onaysız çalışır</option>
          <option value="ask">Sor — dosya yazma/komut için onay iste</option>
          <option value="read-only">Salt-okunur — yazma/komut engellenir</option>
        </select>
      </Field>

      <div className="flex items-center gap-1.5 pt-2 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
        <Sparkles size={13} className="text-[var(--color-accent)]" /> Anthropic
      </div>
      <ProviderKeyField
        label="API anahtarı"
        isSet={draft.anthropicKeySet}
        requiredHint="anthropic sağlayıcısı için gerekli."
        secrets={secrets}
        onPick={async (n) => applyKey('anthropic', await onImportSecret(n))}
        onClear={() => clearKey('anthropic')}
        onManage={onManageSecrets}
      />
      <Field label="claude CLI yolu" hint="Boş = PATH üzerinden otomatik tespit.">
        <input value={draft.claudeCliPath} onChange={(e) => set('claudeCliPath', e.target.value)} placeholder="otomatik" className={inputCls} />
      </Field>
      <TestConnection
        test={test}
        runTest={runTest}
        provider="anthropic"
        disabled={!draft.anthropicKeySet}
        disabledHint="önce Anthropic anahtarı ekle"
      />

      <div className="flex items-center gap-1.5 pt-2 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
        <Zap size={13} className="text-[var(--color-accent)]" /> MiniMax (OpenAI-uyumlu)
      </div>
      <ProviderKeyField
        label="MiniMax API anahtarı"
        isSet={draft.minimaxKeySet}
        requiredHint="MiniMax modelleri için gerekli."
        secrets={secrets}
        onPick={async (n) => applyKey('minimax', await onImportSecret(n))}
        onClear={() => clearKey('minimax')}
        onManage={onManageSecrets}
      />
      <Field label="MiniMax base URL" hint="Boş = https://api.minimax.io/v1 (OpenAI-uyumlu uç).">
        <input value={draft.minimaxBaseUrl} onChange={(e) => set('minimaxBaseUrl', e.target.value)} placeholder="https://api.minimax.io/v1" className={inputCls} />
      </Field>
      <TestConnection
        test={test}
        runTest={runTest}
        provider="minimax"
        model="MiniMax-M3"
        disabled={!draft.minimaxKeySet}
        disabledHint="önce MiniMax anahtarı ekle"
      />

      <div className="flex items-center gap-1.5 pt-2 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
        <Network size={13} className="text-[var(--color-accent)]" /> OpenRouter (OpenAI-uyumlu)
      </div>
      <ProviderKeyField
        label="OpenRouter API anahtarı"
        isSet={draft.openrouterKeySet}
        requiredHint="OpenRouter modelleri için gerekli (tek anahtar, yüzlerce model)."
        secrets={secrets}
        onPick={async (n) => applyKey('openrouter', await onImportSecret(n))}
        onClear={() => clearKey('openrouter')}
        onManage={onManageSecrets}
      />
      <Field label="OpenRouter base URL" hint="Boş = https://openrouter.ai/api/v1 (OpenAI-uyumlu uç).">
        <input value={draft.openrouterBaseUrl} onChange={(e) => set('openrouterBaseUrl', e.target.value)} placeholder="https://openrouter.ai/api/v1" className={inputCls} />
      </Field>
      <TestConnection
        test={test}
        runTest={runTest}
        provider="openrouter"
        model="anthropic/claude-sonnet-4.6"
        disabled={!draft.openrouterKeySet}
        disabledHint="önce OpenRouter anahtarı ekle"
      />

      <div className="flex items-center gap-1.5 pt-2 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
        <Boxes size={13} className="text-[var(--color-accent)]" /> Özel sağlayıcılar
      </div>
      <p className="-mt-1 text-xs text-[var(--color-text-dim)]">OpenAI- veya Anthropic-uyumlu herhangi bir uç (OpenRouter, Gemini, Kimi, Ollama…). Eklenince ajan oluştururken sağlayıcı olarak seçilebilir. Değişiklikler anında kaydedilir (üstteki Kaydet'ten bağımsız).</p>
      <CustomProviders secrets={secrets} onImportSecret={onImportSecret} onManageSecrets={onManageSecrets} />
    </>
  )
}
