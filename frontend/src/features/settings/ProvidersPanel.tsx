// Providers category: default provider/model picker, Anthropic + MiniMax +
// OpenRouter keys, provider instances (kind-driven, generic form) and
// connection test. (Anthropic beta toggles now live under the "Bağlam &
// Bellek" category.)
//
// Layout is deliberately table-like: the three built-in providers render as a
// uniform grid of cards (status badge + aligned key/endpoint columns + test) so
// the section reads as rows of the same shape, and provider instances render
// as a real list (ProviderInstanceList).
import { useState } from 'react'
import { Sparkles, Zap, KeyRound, Boxes, Network, Terminal, type LucideIcon } from 'lucide-react'
import { api } from '@/api'
import { toast } from '@/shared/components'
import type { AppSettings, ProviderTestResult, Secret } from '@/types'
import type { ProviderInstance } from '@/api/providers'
import { useCatalog, resolveRuntimeBadge } from '@/shared/lib/catalog'
import { modelDisplayName } from '@/shared/lib/modelLabel'
import { inputCls, type AppSet } from './primitives'
import { ClaudeAuthDialog } from './ClaudeAuthDialog'
import { CodexAuthDialog } from './CodexAuthDialog'
import { useProviderInstances } from './providers/useProviderInstances'
import { ProviderInstanceList } from './providers/ProviderInstanceList'
import { ProviderInstanceForm } from './providers/ProviderInstanceForm'

function testBadge(test: Props['test'], provider: string) {
  const r = test[provider]
  if (!r) return null
  if (r === 'pending')
    return <span className="text-xs text-[var(--color-warning)]">test ediliyor…</span>
  if (r.ok)
    return (
      <span className="text-xs text-[var(--color-success)]">
        ✓ bağlandı
        {r.model ? <span title={r.model}> ({modelDisplayName(r.model)})</span> : ''}
      </span>
    )
  return <span className="text-xs text-[var(--color-danger)]">✗ {r.error}</span>
}

interface Props {
  draft: AppSettings
  set: AppSet
  setDraft: React.Dispatch<React.SetStateAction<AppSettings | null>>
  test: Record<string, ProviderTestResult | 'pending'>
  runTest: (provider: string, model?: string) => void
  clearKey: (which: 'anthropic' | 'minimax' | 'openrouter' | 'zai' | 'deepseek') => void
  // Apply a provider key immediately (resolved from a vault secret). Provider
  // keys are never typed — only selected from the secret store.
  applyKey: (
    which: 'anthropic' | 'minimax' | 'openrouter' | 'zai' | 'deepseek',
    value: string,
  ) => void | Promise<void>
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
  // Active workspace's resolved codex-cli config home (<workspace>/codex-home),
  // same reasoning as workspaceClaudeHome above.
  workspaceCodexHome?: string
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
          <option key={s.name} value={s.name}>
            {s.name}
          </option>
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
          <span className="shrink-0 text-[10px] uppercase tracking-wide text-[var(--color-text-dim)]">
            {kindLabel}
          </span>
        </div>
        <span
          className={`shrink-0 rounded px-1.5 py-0.5 text-[10px] font-medium ${
            isSet
              ? 'bg-[var(--color-surface-2)] text-[var(--color-success)]'
              : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)]'
          }`}
        >
          {isSet ? '✓ Anahtar kayıtlı' : 'Anahtar yok'}
        </span>
      </div>

      <div className={hideEndpoint ? 'grid gap-2' : 'grid gap-2 sm:grid-cols-2'}>
        <div className="flex flex-col gap-1">
          <span className="text-xs font-medium text-[var(--color-text-dim)]">{keyLabel}</span>
          <KeyPicker
            isSet={isSet}
            secrets={secrets}
            onPick={onPick}
            onClear={onClear}
            provider={testProvider}
          />
        </div>
        {!hideEndpoint && (
          <div className="flex flex-col gap-1">
            <span className="text-xs font-medium text-[var(--color-text-dim)]">
              {endpointLabel}
            </span>
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
            onChange={(e) => {
              if (!endpoint2ReadOnly) onEndpoint2(e.target.value)
            }}
            readOnly={endpoint2ReadOnly}
            placeholder={endpoint2Placeholder}
            className={endpoint2ReadOnly ? `${inputCls} cursor-not-allowed opacity-60` : inputCls}
          />
          {endpoint2Hint && (
            <span className="text-[10px] text-[var(--color-text-dim)]">{endpoint2Hint}</span>
          )}
        </div>
      )}

      {extra}

      <div className="flex items-center justify-between gap-2">
        <span className="min-w-0 truncate text-[11px] text-[var(--color-text-dim)]">
          {isSet ? '✓ Kayıtlı (şifreli).' : requiredHint}
        </span>
        <div className="flex shrink-0 items-center gap-2">
          {!isSet ? (
            <span className="text-xs text-[var(--color-text-dim)]">{testDisabledHint}</span>
          ) : (
            testBadge(test, testProvider)
          )}
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

// ProviderInstances lists every configured provider instance (kind-driven,
// generic form) and hosts the add/edit form. Fetches and mutates via the
// dedicated /api/providers + /api/provider-kinds endpoints, independent of
// the main settings save flow.
function ProviderInstances() {
  const { kinds, instances, loading, error, upsert, remove } = useProviderInstances()
  const [editing, setEditing] = useState<ProviderInstance | null>(null)
  const [adding, setAdding] = useState(false)

  if (loading) {
    return <p className="text-xs text-[var(--color-text-dim)]">Yükleniyor…</p>
  }
  if (error) {
    return <p className="text-xs text-[var(--color-danger)]">{error}</p>
  }

  const startEdit = (inst: ProviderInstance) => {
    setEditing(inst)
    setAdding(false)
  }
  const startAdd = () => {
    setEditing(null)
    setAdding(true)
  }
  const cancel = () => {
    setEditing(null)
    setAdding(false)
  }

  const handleDelete = async (inst: ProviderInstance) => {
    if (!confirm(`"${inst.label || inst.id}" sağlayıcı örneğini silmek istediğine emin misin?`)) {
      return
    }
    try {
      const result = await remove(inst.id)
      if (result.affectedAgents.length > 0) {
        toast.error(
          `Sağlayıcı silindi, ancak ${result.affectedAgents.length} ajan hâlâ bu örneğe bağlıydı. Bu ajanları yeniden yapılandır.`,
        )
      } else {
        toast.success('Sağlayıcı örneği silindi')
      }
      if (editing?.id === inst.id) cancel()
    } catch (e) {
      toast.error((e as Error).message)
    }
  }

  return (
    <div className="space-y-2">
      <ProviderInstanceList
        instances={instances}
        kinds={kinds}
        onEdit={startEdit}
        onDelete={handleDelete}
      />

      {editing || adding ? (
        <ProviderInstanceForm
          kinds={kinds}
          instances={instances}
          editing={editing}
          onCancel={cancel}
          onSave={async (input) => {
            await upsert(input)
            toast.success(editing ? 'Sağlayıcı örneği güncellendi' : 'Sağlayıcı örneği eklendi')
            cancel()
          }}
        />
      ) : (
        <button
          data-testid="provider-instance-add"
          onClick={startAdd}
          className="rounded border border-dashed border-[var(--color-border)] px-3 py-1.5 text-xs hover:border-[var(--color-accent)]"
        >
          + Yeni sağlayıcı örneği
        </button>
      )}
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
  workspaceCodexHome,
}: Props) {
  const [authOpen, setAuthOpen] = useState(false)
  const [codexAuthOpen, setCodexAuthOpen] = useState(false)
  const catalog = useCatalog()
  // Which Claude Code binary + plan actually backs the claude-cli card. Comes
  // from the catalog (the backend probes `claude --version` and reads the
  // workspace claude-home login), so the card names the install, not just "CLI".
  const claudeRuntime = resolveRuntimeBadge(catalog.find((c) => c.id === 'claude-cli'))
  // Same idea for codex-cli: backend probes `codex --version` and checks for a
  // readable auth.json in the workspace codex-home (see catalog_codexcli.go).
  const codexRuntime = resolveRuntimeBadge(catalog.find((c) => c.id === 'codex-cli'))
  // Pre-flight login check for THIS workspace's claude-home (distinct from the
  // generic "test et", which probes the app-global config dir). 'idle' before run.
  const [wsAuth, setWsAuth] = useState<'idle' | 'pending' | { loggedIn: boolean; detail?: string }>(
    'idle',
  )
  const checkWsAuth = async () => {
    setWsAuth('pending')
    try {
      const r = await api.checkWorkspaceClaudeAuth()
      setWsAuth({ loggedIn: r.loggedIn, detail: r.detail })
    } catch (e) {
      setWsAuth({ loggedIn: false, detail: (e as Error).message })
    }
  }
  // Same pre-flight idea as wsAuth above, but for THIS workspace's codex-home
  // (cheap filesystem check server-side, see codex_auth.go).
  const [codexAuth, setCodexAuth] = useState<
    'idle' | 'pending' | { loggedIn: boolean; detail?: string }
  >('idle')
  const checkCodexAuth = async () => {
    setCodexAuth('pending')
    try {
      const r = await api.checkWorkspaceCodexAuth()
      setCodexAuth({ loggedIn: r.loggedIn, detail: r.detail })
    } catch (e) {
      setCodexAuth({ loggedIn: false, detail: (e as Error).message })
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
      {codexAuthOpen && (
        <CodexAuthDialog
          isLoggedIn={typeof codexAuth === 'object' && codexAuth.loggedIn}
          onClose={() => setCodexAuthOpen(false)}
          onLoggedIn={checkCodexAuth}
        />
      )}
      <div>
        <div className="mb-1.5 flex items-center justify-between gap-2">
          <span className="text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
            Yerleşik sağlayıcılar
          </span>
          <button
            onClick={onManageSecrets}
            className="flex items-center gap-1 text-xs text-[var(--color-text-dim)] hover:text-[var(--color-accent)]"
          >
            <KeyRound size={12} /> Sırları yönet →
          </button>
        </div>
        <p className="mb-2 text-xs text-[var(--color-text-dim)]">
          Anahtarlar yalnızca Sır kasasından seçilir — elle giriş kapalı.
        </p>
        <div className="grid gap-2">
          <div className="space-y-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
            <div className="flex items-center justify-between gap-2">
              <div className="flex min-w-0 items-center gap-2">
                <Sparkles size={15} className="shrink-0 text-[var(--color-accent)]" />
                <span className="truncate text-sm font-medium">Anthropic Pro/Max (OAuth)</span>
                <span className="shrink-0 text-[10px] uppercase tracking-wide text-[var(--color-text-dim)]">
                  claude-cli / abonelik
                </span>
                {claudeRuntime && (
                  <span
                    data-testid="claude-cli-runtime"
                    title="Kurulu Claude Code sürümü ve bu workspace'in giriş yaptığı plan"
                    className="shrink-0 rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-[10px] font-medium text-[var(--color-text)]"
                  >
                    {claudeRuntime}
                  </span>
                )}
              </div>
              <span
                className={`shrink-0 rounded px-1.5 py-0.5 text-[10px] font-medium ${
                  draft.claudeCliAuthSet
                    ? 'bg-[var(--color-surface-2)] text-[var(--color-success)]'
                    : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)]'
                }`}
              >
                {draft.claudeCliAuthSet
                  ? `✓ ${draft.claudeCliAuthKind === 'apikey' ? 'API token' : 'Max/Pro token'} kayıtlı`
                  : 'Kimlik yok'}
              </span>
            </div>

            <div className="flex flex-col gap-1">
              <span className="text-xs font-medium text-[var(--color-text-dim)]">
                claude CLI yolu
              </span>
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
              <span className="text-xs font-medium text-[var(--color-text-dim)]">
                claude config dizini (bu workspace · salt-okunur)
              </span>
              <input
                value={workspaceClaudeHome || draft.claudeConfigDir}
                readOnly
                placeholder="per-workspace: <workspace>/claude-home"
                className={`${inputCls} cursor-not-allowed opacity-60`}
              />
              <span className="text-[10px] text-[var(--color-text-dim)]">
                {workspaceClaudeHome
                  ? "Aktif workspace'in kendi CLAUDE_CONFIG_DIR yolu — skill/ayar/login bu workspace ile paylaşılır. Her workspace farklı bir yol kullanır; salt-okunur (workspace kökünden türetilir)."
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

          <div className="space-y-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
            <div className="flex items-center justify-between gap-2">
              <div className="flex min-w-0 items-center gap-2">
                <Terminal size={15} className="shrink-0 text-[var(--color-accent)]" />
                <span className="truncate text-sm font-medium">ChatGPT / Codex (abonelik)</span>
                <span className="shrink-0 text-[10px] uppercase tracking-wide text-[var(--color-text-dim)]">
                  codex-cli / abonelik
                </span>
                {codexRuntime && (
                  <span
                    data-testid="codex-cli-runtime"
                    title="Kurulu Codex CLI sürümü ve bu workspace'in codex-home'unda okunan giriş durumu"
                    className="shrink-0 rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-[10px] font-medium text-[var(--color-text)]"
                  >
                    {codexRuntime}
                  </span>
                )}
              </div>
            </div>

            <div className="flex flex-col gap-1">
              <span className="text-xs font-medium text-[var(--color-text-dim)]">
                codex CLI yolu
              </span>
              <input
                data-testid="provider-endpoint-input"
                data-provider="codex-cli"
                value={draft.codexCliPath}
                onChange={(e) => set('codexCliPath', e.target.value)}
                placeholder="otomatik (PATH)"
                className={inputCls}
              />
            </div>

            <div className="flex flex-col gap-1">
              <span className="text-xs font-medium text-[var(--color-text-dim)]">
                codex config dizini (bu workspace · salt-okunur)
              </span>
              <input
                value={workspaceCodexHome || draft.codexConfigDir}
                readOnly
                placeholder="per-workspace: <workspace>/codex-home"
                className={`${inputCls} cursor-not-allowed opacity-60`}
              />
              <span className="text-[10px] text-[var(--color-text-dim)]">
                {workspaceCodexHome
                  ? "Aktif workspace'in kendi CODEX_HOME yolu — login/config.toml bu workspace ile paylaşılır. Her workspace farklı bir yol kullanır; salt-okunur (workspace kökünden türetilir)."
                  : 'Uygulama-geneli fallback (workspace çözülemedi). Normalde her workspace kendi <workspace>/codex-home dizinini kullanır; salt-okunur.'}
              </span>
            </div>

            <div className="space-y-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] p-2.5">
              <div className="flex flex-wrap items-center gap-2">
                <button
                  data-testid="codex-auth-open"
                  onClick={() => setCodexAuthOpen(true)}
                  className="flex items-center gap-1.5 rounded-lg border border-[var(--color-border)] px-2.5 py-1.5 text-xs hover:border-[var(--color-accent)]"
                >
                  <KeyRound size={12} /> codex-cli kimlik (tarayıcı / API)
                </button>
                <span className="text-[11px] text-[var(--color-text-dim)]">
                  Bu workspace'in izole codex-home'una giriş yap — terminal gerekmez.
                </span>
              </div>
              {/* Pre-flight: verify THIS workspace's codex-home is logged in
                  before an agent turn hits an auth wall. Cheap filesystem probe. */}
              <div className="flex flex-wrap items-center gap-2">
                <button
                  data-testid="workspace-codex-auth-check"
                  onClick={checkCodexAuth}
                  className="flex items-center gap-1.5 rounded-lg border border-[var(--color-border)] px-2.5 py-1.5 text-xs hover:border-[var(--color-accent)]"
                >
                  <KeyRound size={12} /> Bu workspace login doğrula
                </button>
                {codexAuth === 'pending' && (
                  <span className="text-[11px] text-[var(--color-warning)]">kontrol ediliyor…</span>
                )}
                {typeof codexAuth === 'object' && codexAuth.loggedIn && (
                  <span className="text-[11px] text-[var(--color-success)]">✓ giriş yapılmış</span>
                )}
                {typeof codexAuth === 'object' && !codexAuth.loggedIn && (
                  <span className="text-[11px] text-[var(--color-error)]" title={codexAuth.detail}>
                    ✕ giriş yok — {codexAuth.detail || 'codex girişi gerekli'}
                  </span>
                )}
              </div>
              {/* Manual PowerShell fallback — secondary now that the dialog above
                  covers login. Kept for when global ~/.codex isn't seeded and the
                  in-app device/API-key flow isn't preferred. */}
              <details className="text-[11px] text-[var(--color-text-dim)]">
                <summary className="cursor-pointer select-none hover:text-[var(--color-text)]">
                  Gerekirse: elle PowerShell ile giriş
                </summary>
                <div className="mt-1.5 space-y-1">
                  <p>
                    Her workspace açılışında, bu workspace'in codex-home'u henüz giriş yapılmamışsa
                    global ~/.codex (veya $CODEX_HOME) girişi otomatik olarak buraya kopyalanır. Bu
                    komut yalnızca hiçbir global girişin bulunmadığı durumda gerekir:
                  </p>
                  <code className="block overflow-x-auto whitespace-pre rounded bg-[var(--color-surface-2)] px-2 py-1">
                    {`$env:CODEX_HOME = "${workspaceCodexHome || '<workspace>/codex-home'}"\ncodex login`}
                  </code>
                  <p>
                    <code className="rounded bg-[var(--color-surface-2)] px-1 py-0.5">codex</code>{' '}
                    komutu PATH'te olmayabilir — resmi Windows kurulumu ikili dosyayı genellikle{' '}
                    <code className="rounded bg-[var(--color-surface-2)] px-1 py-0.5">
                      %LOCALAPPDATA%\Programs\OpenAI\Codex\bin\codex.exe
                    </code>{' '}
                    konumuna kurar ve PATH'e eklemez.{' '}
                    <code className="rounded bg-[var(--color-surface-2)] px-1 py-0.5">codex</code>{' '}
                    bulunamıyorsa yukarıdaki komutta tam yolu kullan.
                  </p>
                </div>
              </details>
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
          <BuiltinProvider
            icon={Network}
            name="Z.ai GLM"
            kindLabel="Anthropic-uyumlu"
            keyLabel="API anahtarı"
            isSet={draft.zaiKeySet}
            requiredHint="GLM modelleri için gerekli (Anthropic modu: araç kullanımı + düşünme)."
            secrets={secrets}
            onPick={async (n) => applyKey('zai', await onImportSecret(n))}
            onClear={() => clearKey('zai')}
            endpointLabel="base URL"
            endpointValue={draft.zaiBaseUrl}
            endpointPlaceholder="https://api.z.ai/api/anthropic/v1/messages"
            onEndpoint={(v) => set('zaiBaseUrl', v)}
            test={test}
            runTest={runTest}
            testProvider="zai"
            testModel="glm-5.2"
            testDisabledHint="önce Z.ai anahtarı ekle"
          />
          <BuiltinProvider
            icon={Zap}
            name="DeepSeek"
            kindLabel="OpenAI-uyumlu"
            keyLabel="API anahtarı"
            isSet={draft.deepseekKeySet}
            requiredHint="DeepSeek V4 modelleri için gerekli (1M bağlam, çok ucuz)."
            secrets={secrets}
            onPick={async (n) => applyKey('deepseek', await onImportSecret(n))}
            onClear={() => clearKey('deepseek')}
            endpointLabel="base URL"
            endpointValue={draft.deepseekBaseUrl}
            endpointPlaceholder="https://api.deepseek.com"
            onEndpoint={(v) => set('deepseekBaseUrl', v)}
            test={test}
            runTest={runTest}
            testProvider="deepseek"
            testModel="deepseek-v4-flash"
            testDisabledHint="önce DeepSeek anahtarı ekle"
          />
        </div>
      </div>

      <div className="flex items-center gap-1.5 pt-2 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
        <Boxes size={13} className="text-[var(--color-accent)]" /> Sağlayıcı örnekleri
      </div>
      <p className="-mt-1 text-xs text-[var(--color-text-dim)]">
        Kayıtlı her taslaktan (kind) birden fazla örnek oluşturulabilir (farklı token/config taşıyan
        aynı sağlayıcı, ör. iki ayrı Anthropic hesabı). Eklenince ajan oluştururken sağlayıcı olarak
        seçilebilir. Değişiklikler anında kaydedilir (üstteki Kaydet'ten bağımsız).
      </p>
      <ProviderInstances />
    </>
  )
}
