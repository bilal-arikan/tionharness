import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { Bell, Tag, type LucideIcon } from 'lucide-react'
import { api } from '@/api'
import type {
  AppSettings,
  PromptInfo,
  ProviderTestResult,
  SettingsPatch,
  SlashCommand,
} from '@/types'
import { LoadingState, toast } from '@/shared/components'
import { CatButton, type Cat } from './primitives'
import { APP_CATS } from './settingsCats'
import {
  ProfilePanel,
  NotificationsPanel,
  SoundPanel,
  ContextPanel,
  AutoTitlePanel,
  ToolsPanel,
  BackupPanel,
  AboutPanel,
} from './appPanels'
import type { DesktopNotificationsMode } from '@/types/workspace'
import { useRegisterDirty } from '@/shared/lib/dirtySignals'
import { Button, CollapsibleListShell } from '@/shared/components'
import { ProvidersPanel } from './ProvidersPanel'
import { CommandsPanel } from './CommandsPanel'
import { StepKindsPanel } from './StepKindsPanel'
import { HooksPanel } from './HooksPanel'
import { ExternalToolsPanel } from './ExternalToolsPanel'
import { SecretsPanel } from './SecretsPanel'

interface Props {
  onError: (msg: string) => void
  // Re-apply theme/accent globally after an app-settings save.
  onSaved: (s: AppSettings) => void
  // Re-resolve the effective desktop-notification gate after the active
  // workspace's three-state override is saved from the Notifications panel, so it
  // applies live without a workspace switch or a global-settings reload.
  onWorkspaceNotifySaved?: (mode: DesktopNotificationsMode) => void
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
  // Left category rail collapse — controlled from the app header's list toggle,
  // matching every other list screen. Defaults to open when not provided.
  navOpen?: boolean
  onToggleNav?: () => void
}

const ALL_CATS: Cat[] = APP_CATS.map((c) => c.key)
function isCat(v: string | null | undefined): v is Cat {
  return !!v && (ALL_CATS as string[]).includes(v)
}

// SettingsPanel is the two-pane configuration screen: a category rail on the
// left (like the chat session list) and the selected category's fields on the
// right. App-global settings and per-workspace settings are separate scopes.
// The per-category forms live in ./settings/*.
export function SettingsPanel({
  onError,
  onSaved,
  onWorkspaceNotifySaved,
  commands = [],
  cat: catProp,
  onCatChange,
  reloadNonce = 0,
  navOpen,
  onToggleNav,
}: Props) {
  // Category is controlled by the parent (URL deep-link) when onCatChange is
  // given; an unknown/empty routed category falls back to 'profile'.
  const [catState, setCatState] = useState<Cat>('profile')
  const controlled = onCatChange !== undefined
  const cat: Cat = controlled ? (isCat(catProp) ? catProp : 'profile') : catState
  const setCat = (c: Cat) => (onCatChange ? onCatChange(c) : setCatState(c))

  // App-global settings scope.
  const [draft, setDraft] = useState<AppSettings | null>(null)
  const [original, setOriginal] = useState<AppSettings | null>(null)
  const [test, setTest] = useState<Record<string, ProviderTestResult | 'pending'>>({})
  // Active workspace's resolved claude-cli config home (<workspace>/claude-home),
  // shown read-only in the Providers panel. Per-workspace, unlike the app-global
  // claudeConfigDir fallback — fetched from the workspace-settings endpoint.
  const [wsClaudeHome, setWsClaudeHome] = useState('')
  // Active workspace's resolved codex-cli config home (<workspace>/codex-home),
  // same reasoning as wsClaudeHome above.
  const [wsCodexHome, setWsCodexHome] = useState('')

  const [saving, setSaving] = useState(false)

  // Built-in runtime prompts (read-only) shown in the Komutlar category.
  const [prompts, setPrompts] = useState<PromptInfo[]>([])
  const [promptsDir, setPromptsDir] = useState('')
  // Which command/prompt cards are expanded (name → open) in the Komutlar list.
  const [openCmds, setOpenCmds] = useState<Record<string, boolean>>({})

  useEffect(() => {
    api
      .getSettings()
      .then((s) => {
        setDraft(s)
        setOriginal(s)
      })
      .catch((e) => onError((e as Error).message))
    api
      .getPrompts()
      .then((p) => {
        setPrompts(p.prompts)
        setPromptsDir(p.dir)
      })
      .catch(() => {})
    // Active workspace's real claude-home path for the read-only Providers field.
    api
      .getWorkspaceSettings()
      .then((w) => {
        setWsClaudeHome(w.claudeHomeDir)
        setWsCodexHome(w.codexHomeDir)
      })
      .catch(() => {})
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const dirtyApp = useMemo(
    () => !!(draft && original && JSON.stringify(draft) !== JSON.stringify(original)),
    [draft, original],
  )
  const dirty = dirtyApp
  // Surface unsaved settings on the nav "Ayarlar" item + workspace label.
  useRegisterDirty('settings', dirty)

  // Live reload: when an agent changes app settings (parent bumps reloadNonce),
  // re-fetch and refresh the form — but skip while the user has unsaved edits so
  // their in-progress changes are never clobbered. reloadNonce starts at 0; the
  // first bump (>0) is the first real signal.
  useEffect(() => {
    if (reloadNonce === 0 || dirtyApp) return
    api
      .getSettings()
      .then((s) => {
        setDraft(s)
        setOriginal(s)
      })
      .catch(() => {})
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [reloadNonce])

  const set = <K extends keyof AppSettings>(key: K, val: AppSettings[K]) =>
    setDraft((d) => (d ? { ...d, [key]: val } : d))

  const saveApp = async () => {
    if (!draft) return
    const patch: SettingsPatch = {
      theme: draft.theme,
      accent: draft.accent,
      themePreset: draft.themePreset,
      language: draft.language,
      uiLanguage: draft.uiLanguage,
      defaultPermissionMode: draft.defaultPermissionMode,
      claudeConfigDir: draft.claudeConfigDir,
      extendedPromptCache: draft.extendedPromptCache,
      anthropicContextEditing: draft.anthropicContextEditing,
      anthropicNativeToolSearch: draft.anthropicNativeToolSearch,
      anthropicProgrammaticTools: draft.anthropicProgrammaticTools,
      anthropicWebTools: draft.anthropicWebTools,
      anthropicServerCompaction: draft.anthropicServerCompaction,
      anthropicRefusalFallback: draft.anthropicRefusalFallback,
      autonomousTaskBudgetTokens: draft.autonomousTaskBudgetTokens,
      desktopNotifications: draft.desktopNotifications,
      keepAwake: draft.keepAwake,
      userName: draft.userName,
      userTimezone: draft.userTimezone,
      userCity: draft.userCity,
      userCountry: draft.userCountry,
      userNotes: draft.userNotes,
      maxContextTokens: draft.maxContextTokens,
      keepRecentMsgs: draft.keepRecentMsgs,
      contextBudgetCeil: draft.contextBudgetCeil,
      contextBudgetFraction: draft.contextBudgetFraction,
      reactiveCompact: draft.reactiveCompact,
      maxTokenRetries: draft.maxTokenRetries,
      reactiveKeepRecent: draft.reactiveKeepRecent,
      maxOutputTokens: draft.maxOutputTokens,
      maxProviderRetries: draft.maxProviderRetries,
      // Self-healing (guardrails + stuck threshold + lessonReflect) moved to
      // İçgörü ▸ Öz-iyileşme; NOT patched here so a Settings save can't clobber a
      // change made there with this panel's stale draft.
      handoffAuto: draft.handoffAuto,
      handoffMaxChain: draft.handoffMaxChain,
      handoffWriteFile: draft.handoffWriteFile,
      progressPersist: draft.progressPersist,
      progressResume: draft.progressResume,
      autoTagSessions: draft.autoTagSessions,
      debugJournalEnabled: draft.debugJournalEnabled,
      debugJournalCap: draft.debugJournalCap,
      autoTitleEnabled: draft.autoTitleEnabled,
      titleModel: draft.titleModel,
      titleProviderId: draft.titleProviderId,
      enableShell: draft.enableShell,
      enableCliHooks: draft.enableCliHooks,
      enableCodeMode: draft.enableCodeMode,
      claudeResume: draft.claudeResume,
      claudePersistentSession: draft.claudePersistentSession,
      claudeSysPromptFile: draft.claudeSysPromptFile,
      delegationMaxDepth: draft.delegationMaxDepth,
      delegationMaxCalls: draft.delegationMaxCalls,
      spawnMaxConcurrent: draft.spawnMaxConcurrent,
      spawnQueueMax: draft.spawnQueueMax,
      spawnMaxPerTurn: draft.spawnMaxPerTurn,
      spawnTimeoutMin: draft.spawnTimeoutMin,
      spawnIdleTimeoutMin: draft.spawnIdleTimeoutMin,
      chatTurnTimeoutMin: draft.chatTurnTimeoutMin,
      chatTurnIdleTimeoutMin: draft.chatTurnIdleTimeoutMin,
      idleResumeMax: draft.idleResumeMax,
      scheduleTimeoutMin: draft.scheduleTimeoutMin,
      turnWatchdogMin: draft.turnWatchdogMin,
      turnIdleWatchdogMin: draft.turnIdleWatchdogMin,
      shellDefaultTimeoutSec: draft.shellDefaultTimeoutSec,
      shellMaxTimeoutSec: draft.shellMaxTimeoutSec,
      maxToolOutputKB: draft.maxToolOutputKB,
      coordinatorMaxWorkers: draft.coordinatorMaxWorkers,
      coordinatorMaxTurns: draft.coordinatorMaxTurns,
      coordinatorMaxDepth: draft.coordinatorMaxDepth,
      coordinatorMaxSubtreeSessions: draft.coordinatorMaxSubtreeSessions,
      coordinatorSettleGraceSec: draft.coordinatorSettleGraceSec,
      coordinatorStallGuard: draft.coordinatorStallGuard,
      coordinatorStallSweepMin: draft.coordinatorStallSweepMin,
      coordinatorStallMaxNudges: draft.coordinatorStallMaxNudges,
      autonomousConfine: draft.autonomousConfine,
      autonomousBootSeq: draft.autonomousBootSeq,
      backupEnabled: draft.backupEnabled,
      backupIntervalHours: draft.backupIntervalHours,
      backupRetain: draft.backupRetain,
      backupDir: draft.backupDir,
    }
    const updated = await api.updateSettings(patch)
    setDraft(updated)
    setOriginal(updated)
    onSaved(updated)
  }

  const save = async () => {
    setSaving(true)
    try {
      await saveApp()
      toast.success('Kaydedildi')
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setSaving(false)
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
      {/* Left: category rail (collapsible via the app header toggle; mobile drawer) */}
      <CollapsibleListShell
        open={navOpen ?? true}
        onToggle={onToggleNav ?? (() => {})}
        label="Ayarlar"
        hideRail
      >
        <aside className="flex h-full w-56 flex-shrink-0 flex-col gap-1 overflow-y-auto border-r border-[var(--color-border)] bg-[var(--color-surface)] p-2 max-md:w-[85vw] max-md:max-w-sm">
          <div className="px-2 pb-1 pt-2 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
            Uygulama
          </div>
          {APP_CATS.map((c) => (
            <CatButton
              key={c.key}
              c={c}
              active={cat === c.key}
              onClick={() => setCat(c.key)}
              dirty={c.key !== 'about' && c.key !== 'secrets' && !!dirtyApp}
            />
          ))}
        </aside>
      </CollapsibleListShell>

      {/* Right: content for the active category. min-w-0 lets this flex column
          shrink below its content's intrinsic width on narrow screens (otherwise
          a wide child — e.g. a Hooks/ExternalTools code sample — forces overflow). */}
      <div className="flex min-w-0 flex-1 flex-col">
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
            {cat !== 'secrets' && cat !== 'exttools' && (
              <span className="text-xs text-[var(--color-text-dim)]">
                {dirty ? 'Kaydedilmemiş değişiklik' : 'Kayıtlı'}
              </span>
            )}
            {cat !== 'about' &&
              cat !== 'commands' &&
              cat !== 'stepkinds' &&
              cat !== 'hooks' &&
              cat !== 'exttools' &&
              cat !== 'secrets' && (
                <Button onClick={save} disabled={!dirty || saving}>
                  {saving ? 'Kaydediliyor…' : 'Kaydet'}
                </Button>
              )}
          </div>
        </div>

        {cat === 'secrets' ? (
          // Secrets manages its own list/forms; render full-bleed (the tool
          // catalog "Araçlar & MCP" now lives as a top-level NavRail view).
          <SecretsPanel onError={onError} />
        ) : (
          <div className="mx-auto w-full max-w-2xl flex-1 space-y-4 overflow-y-auto p-6">
            {!draft ? (
              <LoadingState label="Yükleniyor…" />
            ) : (
              <>
                {cat === 'profile' && <ProfilePanel draft={draft} set={set} setDraft={setDraft} />}
                {cat === 'providers' && (
                  <ProvidersPanel
                    draft={draft}
                    setDraft={setDraft}
                    test={test}
                    runTest={runTest}
                    workspaceClaudeHome={wsClaudeHome}
                    workspaceCodexHome={wsCodexHome}
                  />
                )}
                {cat === 'context' && <ContextPanel draft={draft} set={set} setDraft={setDraft} />}
                {cat === 'tools' && <ToolsPanel draft={draft} set={set} setDraft={setDraft} />}
                {cat === 'backup' && <BackupPanel draft={draft} set={set} setDraft={setDraft} />}
                {cat === 'hooks' && <HooksPanel onError={onError} />}
                {cat === 'exttools' && <ExternalToolsPanel onError={onError} />}
                {cat === 'sound' && <SoundPanel />}
                {cat === 'advanced' && (
                  <>
                    <AdvSection title="Bildirimler & Ekran" icon={Bell}>
                      <NotificationsPanel
                        draft={draft}
                        set={set}
                        setDraft={setDraft}
                        onError={onError}
                        onWorkspaceNotifySaved={onWorkspaceNotifySaved}
                      />
                    </AdvSection>
                    <AdvSection title="Otomatik Başlık" icon={Tag}>
                      <AutoTitlePanel draft={draft} set={set} setDraft={setDraft} />
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
function AdvSection({
  title,
  icon: Icon,
  children,
}: {
  title: string
  icon: LucideIcon
  children: ReactNode
}) {
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
