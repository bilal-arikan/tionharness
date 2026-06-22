import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { Bell, Bot, Tag, Plug, Activity, type LucideIcon } from 'lucide-react'
import { api } from '../api'
import type {
  AppSettings,
  PromptInfo,
  ProviderTestResult,
  Secret,
  SettingsPatch,
  SlashCommand,
} from '../types'
import {
  APP_CATS,
  CatButton,
  type Cat,
} from './settings/primitives'
import {
  ProfilePanel,
  NotificationsPanel,
  AppearancePanel,
  ContextPanel,
  BudgetPanel,
  AutonomyPanel,
  AutoTitlePanel,
  McpPanel,
  ToolsPanel,
  DiagnosticsPanel,
  AboutPanel,
} from './settings/appPanels'
import { ProvidersPanel } from './settings/ProvidersPanel'
import { CommandsPanel } from './settings/CommandsPanel'
import { StepKindsPanel } from './settings/StepKindsPanel'
import { HooksPanel } from './settings/HooksPanel'
import { SecretsPanel } from './panels/SecretsPanel'
// The workspace tool catalog + MCP server management, surfaced here as a
// settings category (previously a top-level NavRail view). Renders its own
// master-detail layout, so it is shown full-bleed below.
import { ToolsPanel as ToolsCatalogPanel } from './panels/ToolsPanel'

interface Props {
  onError: (msg: string) => void
  // Re-apply theme/accent globally after an app-settings save.
  onSaved: (s: AppSettings) => void
  // Slash commands available in the chat composer — shown read-only in the
  // "Komutlar" reference category.
  commands?: SlashCommand[]
  // Controlled active category (deep-link aware). When onCatChange is provided
  // the category is fully controlled by the parent (URL-synced); otherwise it is
  // tracked internally.
  cat?: string | null
  onCatChange?: (c: Cat) => void
  // Bumped by the parent when an agent changes app settings (settings SSE
  // event). On change the app-settings form reloads — but only when it has no
  // unsaved edits, so a concurrent agent change never clobbers in-progress typing.
  reloadNonce?: number
}

const ALL_CATS: Cat[] = APP_CATS.map((c) => c.key)
function isCat(v: string | null | undefined): v is Cat {
  return !!v && (ALL_CATS as string[]).includes(v)
}

// SettingsPanel is the two-pane configuration screen: a category rail on the
// left (like the chat session list) and the selected category's fields on the
// right. App-global settings and per-workspace settings are separate scopes.
// The per-category forms live in ./settings/*.
export function SettingsPanel({ onError, onSaved, commands = [], cat: catProp, onCatChange, reloadNonce = 0 }: Props) {
  // Category is controlled by the parent (URL deep-link) when onCatChange is
  // given; an unknown/empty routed category falls back to 'profile'.
  const [catState, setCatState] = useState<Cat>('profile')
  const controlled = onCatChange !== undefined
  const cat: Cat = controlled ? (isCat(catProp) ? catProp : 'profile') : catState
  const setCat = (c: Cat) => (onCatChange ? onCatChange(c) : setCatState(c))

  // App-global settings scope.
  const [draft, setDraft] = useState<AppSettings | null>(null)
  const [original, setOriginal] = useState<AppSettings | null>(null)
  const [keyInput, setKeyInput] = useState('')
  const [minimaxKeyInput, setMinimaxKeyInput] = useState('')
  const [openrouterKeyInput, setOpenrouterKeyInput] = useState('')
  const [test, setTest] = useState<Record<string, ProviderTestResult | 'pending'>>({})
  // Workspace secret names, offered as an import source for the key fields.
  const [secrets, setSecrets] = useState<Secret[]>([])

  const [saving, setSaving] = useState(false)

  // Built-in runtime prompts (read-only) shown in the Komutlar category.
  const [prompts, setPrompts] = useState<PromptInfo[]>([])
  const [promptsDir, setPromptsDir] = useState('')
  // Which command/prompt cards are expanded (name → open) in the Komutlar list.
  const [openCmds, setOpenCmds] = useState<Record<string, boolean>>({})

  useEffect(() => {
    api.getSettings().then((s) => { setDraft(s); setOriginal(s) }).catch((e) => onError((e as Error).message))
    api.getPrompts().then((p) => { setPrompts(p.prompts); setPromptsDir(p.dir) }).catch(() => {})
    api.listSecrets().then(setSecrets).catch(() => {})
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const dirtyApp = useMemo(
    () =>
      (draft && original && JSON.stringify(draft) !== JSON.stringify(original)) ||
      keyInput.length > 0 ||
      minimaxKeyInput.length > 0 ||
      openrouterKeyInput.length > 0,
    [draft, original, keyInput, minimaxKeyInput, openrouterKeyInput],
  )
  const dirty = dirtyApp

  // Live reload: when an agent changes app settings (parent bumps reloadNonce),
  // re-fetch and refresh the form — but skip while the user has unsaved edits so
  // their in-progress changes are never clobbered. reloadNonce starts at 0; the
  // first bump (>0) is the first real signal.
  useEffect(() => {
    if (reloadNonce === 0 || dirtyApp) return
    api.getSettings().then((s) => { setDraft(s); setOriginal(s) }).catch(() => {})
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [reloadNonce])

  const set = <K extends keyof AppSettings>(key: K, val: AppSettings[K]) =>
    setDraft((d) => (d ? { ...d, [key]: val } : d))

  const saveApp = async () => {
    if (!draft) return
    const patch: SettingsPatch = {
      theme: draft.theme, accent: draft.accent, themePreset: draft.themePreset, language: draft.language,
      defaultProvider: draft.defaultProvider, defaultModel: draft.defaultModel,
      defaultPermissionMode: draft.defaultPermissionMode, claudeCliPath: draft.claudeCliPath,
      minimaxBaseUrl: draft.minimaxBaseUrl,
      openrouterBaseUrl: draft.openrouterBaseUrl,
      extendedPromptCache: draft.extendedPromptCache,
      desktopNotifications: draft.desktopNotifications, keepAwake: draft.keepAwake,
      userName: draft.userName, userTimezone: draft.userTimezone, userCity: draft.userCity,
      userCountry: draft.userCountry, userNotes: draft.userNotes,
      maxContextTokens: draft.maxContextTokens, keepRecentMsgs: draft.keepRecentMsgs,
      recallTopN: draft.recallTopN, recallMinScore: draft.recallMinScore,
      journalCap: draft.journalCap, journalMaxLen: draft.journalMaxLen,
      memoryPressureWarn: draft.memoryPressureWarn, coreMemoryTools: draft.coreMemoryTools,
      autoReflect: draft.autoReflect, autoReflectThreshold: draft.autoReflectThreshold,
      autoUserModel: draft.autoUserModel,
      reactiveCompact: draft.reactiveCompact, maxTokenRetries: draft.maxTokenRetries,
      reactiveKeepRecent: draft.reactiveKeepRecent,
      compactToolOutput: draft.compactToolOutput, compactMaxLines: draft.compactMaxLines,
      compactMaxBytes: draft.compactMaxBytes, compactLlmSummary: draft.compactLlmSummary,
      compactLlmThreshold: draft.compactLlmThreshold, compactModel: draft.compactModel,
      defaultDailyCallLimit: draft.defaultDailyCallLimit, defaultDailyTokenLimit: draft.defaultDailyTokenLimit,
      pauseAutonomy: draft.pauseAutonomy,
      autoTitleEnabled: draft.autoTitleEnabled, titleModel: draft.titleModel,
      mcpGatewayUrl: draft.mcpGatewayUrl, logLevel: draft.logLevel,
      enableShell: draft.enableShell, enableSelfManage: draft.enableSelfManage,
      enableCliHooks: draft.enableCliHooks,
      enableDelegation: draft.enableDelegation,
      delegationMaxDepth: draft.delegationMaxDepth, delegationMaxCalls: draft.delegationMaxCalls,
      spawnMaxConcurrent: draft.spawnMaxConcurrent, spawnMaxPerTurn: draft.spawnMaxPerTurn,
      autonomousConfine: draft.autonomousConfine, gitWorktreeIsolation: draft.gitWorktreeIsolation,
    }
    if (keyInput) patch.anthropicKey = keyInput
    if (minimaxKeyInput) patch.minimaxKey = minimaxKeyInput
    if (openrouterKeyInput) patch.openrouterKey = openrouterKeyInput
    const updated = await api.updateSettings(patch)
    setDraft(updated); setOriginal(updated); setKeyInput(''); setMinimaxKeyInput(''); setOpenrouterKeyInput('')
    onSaved(updated)
  }

  const save = async () => {
    setSaving(true)
    try {
      await saveApp()
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  // keyPatch builds a write-only key patch for the named provider ("" = clear).
  const keyPatch = (which: 'anthropic' | 'minimax' | 'openrouter', value: string): SettingsPatch =>
    which === 'anthropic'
      ? { anthropicKey: value }
      : which === 'minimax'
        ? { minimaxKey: value }
        : { openrouterKey: value }

  const clearKey = async (which: 'anthropic' | 'minimax' | 'openrouter') => {
    try {
      const updated = await api.updateSettings(keyPatch(which, ''))
      setDraft(updated); setOriginal(updated)
      if (which === 'anthropic') setKeyInput('')
      else if (which === 'minimax') setMinimaxKeyInput('')
      else setOpenrouterKeyInput('')
    } catch (e) {
      onError((e as Error).message)
    }
  }

  // applyKey persists a provider key immediately (resolved from a vault secret).
  // Provider keys are never typed — they are only selected from the secret store.
  const applyKey = async (which: 'anthropic' | 'minimax' | 'openrouter', value: string) => {
    if (!value) return
    try {
      const updated = await api.updateSettings(keyPatch(which, value))
      setDraft(updated); setOriginal(updated)
    } catch (e) {
      onError((e as Error).message)
    }
  }

  const runTest = async (provider: string, model?: string) => {
    setTest((t) => ({ ...t, [provider]: 'pending' }))
    try {
      const r = await api.testProvider(provider, model)
      setTest((t) => ({ ...t, [provider]: r }))
    } catch (e) {
      setTest((t) => ({ ...t, [provider]: { ok: false, error: (e as Error).message } }))
    }
  }

  const catMeta = APP_CATS.find((c) => c.key === cat)
  const CatIcon = catMeta?.icon

  return (
    <div className="flex min-h-0 flex-1">
      {/* Left: category rail */}
      <aside className="flex w-56 flex-shrink-0 flex-col gap-1 overflow-y-auto border-r border-[var(--color-border)] bg-[var(--color-surface)] p-2">
        <div className="px-2 pb-1 pt-2 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
          Uygulama
        </div>
        {APP_CATS.map((c) => (
          <CatButton key={c.key} c={c} active={cat === c.key} onClick={() => setCat(c.key)} dirty={c.key !== 'about' && c.key !== 'secrets' && !!dirtyApp} />
        ))}
      </aside>

      {/* Right: content for the active category */}
      <div className="flex flex-1 flex-col">
        <div className="flex items-center justify-between border-b border-[var(--color-border)] px-6 py-3">
          <span className="flex items-center gap-2 text-sm font-semibold">
            {CatIcon && (
              <span className="flex h-6 w-6 items-center justify-center rounded-md bg-[var(--color-accent-soft)] text-[var(--color-accent)]">
                <CatIcon size={14} />
              </span>
            )}
            {catMeta?.label ?? ''}
          </span>
          <div className="flex items-center gap-3">
            {cat !== 'mcptools' && cat !== 'secrets' && (
              <span className="text-xs text-[var(--color-text-dim)]">
                {dirty ? 'Kaydedilmemiş değişiklik' : 'Kayıtlı'}
              </span>
            )}
            {cat !== 'about' && cat !== 'commands' && cat !== 'stepkinds' && cat !== 'hooks' && cat !== 'mcptools' && cat !== 'secrets' && (
              <button
                onClick={save}
                disabled={!dirty || saving}
                className="rounded bg-[var(--color-accent)] px-4 py-1.5 text-sm font-medium text-white hover:opacity-90 disabled:opacity-30"
              >
                {saving ? 'Kaydediliyor…' : 'Kaydet'}
              </button>
            )}
          </div>
        </div>

        {cat === 'mcptools' ? (
          // Tool catalog hosts its own searchable list + detail/server panes, so
          // it is rendered full-bleed (outside the centered max-w content column).
          <ToolsCatalogPanel onError={onError} />
        ) : cat === 'secrets' ? (
          // Secrets manages its own list/forms; render full-bleed like the tool
          // catalog (moved here from a top-level NavRail view).
          <SecretsPanel onError={onError} />
        ) : (
        <div className="mx-auto w-full max-w-2xl flex-1 space-y-4 overflow-y-auto p-6">
          {!draft ? (
            <div className="text-sm text-[var(--color-text-dim)]">Yükleniyor…</div>
          ) : (
            <>
              {cat === 'profile' && <ProfilePanel draft={draft} set={set} setDraft={setDraft} />}
              {cat === 'appearance' && <AppearancePanel draft={draft} set={set} setDraft={setDraft} />}
              {cat === 'providers' && (
                <ProvidersPanel
                  draft={draft}
                  set={set}
                  setDraft={setDraft}
                  test={test}
                  runTest={runTest}
                  clearKey={clearKey}
                  applyKey={applyKey}
                  secrets={secrets}
                  onImportSecret={async (name) => (await api.revealSecret(name)).value}
                  onManageSecrets={() => setCat('secrets')}
                />
              )}
              {cat === 'context' && <ContextPanel draft={draft} set={set} setDraft={setDraft} />}
              {cat === 'budget' && <BudgetPanel draft={draft} set={set} setDraft={setDraft} />}
              {cat === 'tools' && <ToolsPanel draft={draft} set={set} setDraft={setDraft} />}
              {cat === 'hooks' && <HooksPanel onError={onError} />}
              {cat === 'advanced' && (
                <>
                  <AdvSection title="Bildirimler & Ekran" icon={Bell}>
                    <NotificationsPanel draft={draft} set={set} setDraft={setDraft} />
                  </AdvSection>
                  <AdvSection title="Otonomi" icon={Bot}>
                    <AutonomyPanel draft={draft} set={set} setDraft={setDraft} />
                  </AdvSection>
                  <AdvSection title="Otomatik Başlık" icon={Tag}>
                    <AutoTitlePanel draft={draft} set={set} setDraft={setDraft} />
                  </AdvSection>
                  <AdvSection title="MCP & Araçlar" icon={Plug}>
                    <McpPanel draft={draft} set={set} setDraft={setDraft} />
                  </AdvSection>
                  <AdvSection title="Tanılama" icon={Activity}>
                    <DiagnosticsPanel draft={draft} set={set} setDraft={setDraft} />
                  </AdvSection>
                </>
              )}
              {cat === 'commands' && (
                <CommandsPanel
                  commands={commands}
                  prompts={prompts}
                  promptsDir={promptsDir}
                  openCmds={openCmds}
                  setOpenCmds={setOpenCmds}
                  onError={onError}
                />
              )}
              {cat === 'stepkinds' && <StepKindsPanel />}
              {cat === 'about' && <AboutPanel />}
            </>
          )}
        </div>
        )}
      </div>
    </div>
  )
}

// AdvSection groups one former settings category under a labelled sub-header on
// the combined "Gelişmiş" screen, with an accent icon badge and a divider
// between groups.
function AdvSection({ title, icon: Icon, children }: { title: string; icon: LucideIcon; children: ReactNode }) {
  return (
    <section className="space-y-4 border-b border-[var(--color-border)] pb-6 last:border-b-0 last:pb-0">
      <h3 className="flex items-center gap-2 text-sm font-semibold text-[var(--color-text)]">
        <span className="flex h-6 w-6 items-center justify-center rounded-md bg-[var(--color-accent-soft)] text-[var(--color-accent)]">
          <Icon size={14} />
        </span>
        {title}
      </h3>
      {children}
    </section>
  )
}
