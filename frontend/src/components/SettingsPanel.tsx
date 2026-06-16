import { useEffect, useMemo, useState } from 'react'
import { api } from '../api'
import type {
  AppSettings,
  PromptInfo,
  ProviderTestResult,
  SettingsPatch,
  SlashCommand,
  WorkspaceSettings,
} from '../types'
import {
  APP_CATS,
  WS_CATS,
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
  DiagnosticsPanel,
  AboutPanel,
} from './settings/appPanels'
import { ProvidersPanel } from './settings/ProvidersPanel'
import { CommandsPanel } from './settings/CommandsPanel'
import { StepKindsPanel } from './settings/StepKindsPanel'
import { WorkspacePanel } from './settings/WorkspacePanel'
import { WorkspaceFilesPanel } from './settings/WorkspaceFilesPanel'

interface Props {
  onError: (msg: string) => void
  // Re-apply theme/accent globally after an app-settings save.
  onSaved: (s: AppSettings) => void
  // Notify the app that workspace metadata (e.g. name) changed, so the
  // workspace list/switcher can refresh.
  onWorkspaceChanged?: () => void
  // Delete the active workspace (app handles confirm/switch). Returns whether
  // the deletion proceeded.
  onDeleteWorkspace?: () => void
  // Slash commands available in the chat composer — shown read-only in the
  // "Komutlar" reference category.
  commands?: SlashCommand[]
}

// SettingsPanel is the two-pane configuration screen: a category rail on the
// left (like the chat session list) and the selected category's fields on the
// right. App-global settings and per-workspace settings are separate scopes.
// The per-category forms live in ./settings/*.
export function SettingsPanel({ onError, onSaved, onWorkspaceChanged, onDeleteWorkspace, commands = [] }: Props) {
  const [cat, setCat] = useState<Cat>('profile')

  // App-global settings scope.
  const [draft, setDraft] = useState<AppSettings | null>(null)
  const [original, setOriginal] = useState<AppSettings | null>(null)
  const [keyInput, setKeyInput] = useState('')
  const [minimaxKeyInput, setMinimaxKeyInput] = useState('')
  const [test, setTest] = useState<Record<string, ProviderTestResult | 'pending'>>({})

  // Per-workspace settings scope.
  const [ws, setWs] = useState<WorkspaceSettings | null>(null)
  const [wsOrig, setWsOrig] = useState<WorkspaceSettings | null>(null)

  const [saving, setSaving] = useState(false)

  // Built-in runtime prompts (read-only) shown in the Komutlar category.
  const [prompts, setPrompts] = useState<PromptInfo[]>([])
  const [promptsDir, setPromptsDir] = useState('')
  // Which command/prompt cards are expanded (name → open) in the Komutlar list.
  const [openCmds, setOpenCmds] = useState<Record<string, boolean>>({})

  useEffect(() => {
    api.getSettings().then((s) => { setDraft(s); setOriginal(s) }).catch((e) => onError((e as Error).message))
    api.getWorkspaceSettings().then((s) => { setWs(s); setWsOrig(s) }).catch((e) => onError((e as Error).message))
    api.getPrompts().then((p) => { setPrompts(p.prompts); setPromptsDir(p.dir) }).catch(() => {})
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const isWs = cat === 'workspace'
  const dirtyApp = useMemo(
    () =>
      (draft && original && JSON.stringify(draft) !== JSON.stringify(original)) ||
      keyInput.length > 0 ||
      minimaxKeyInput.length > 0,
    [draft, original, keyInput, minimaxKeyInput],
  )
  const dirtyWs = useMemo(
    () => ws && wsOrig && JSON.stringify(ws) !== JSON.stringify(wsOrig),
    [ws, wsOrig],
  )
  const dirty = isWs ? dirtyWs : dirtyApp

  const set = <K extends keyof AppSettings>(key: K, val: AppSettings[K]) =>
    setDraft((d) => (d ? { ...d, [key]: val } : d))
  const setWsField = <K extends keyof WorkspaceSettings>(key: K, val: WorkspaceSettings[K]) =>
    setWs((d) => (d ? { ...d, [key]: val } : d))

  const saveApp = async () => {
    if (!draft) return
    const patch: SettingsPatch = {
      theme: draft.theme, accent: draft.accent, themePreset: draft.themePreset, language: draft.language,
      defaultProvider: draft.defaultProvider, defaultModel: draft.defaultModel, claudeCliPath: draft.claudeCliPath,
      minimaxBaseUrl: draft.minimaxBaseUrl,
      oneMillionContext: draft.oneMillionContext, extendedPromptCache: draft.extendedPromptCache,
      desktopNotifications: draft.desktopNotifications, keepAwake: draft.keepAwake,
      userName: draft.userName, userTimezone: draft.userTimezone, userCity: draft.userCity,
      userCountry: draft.userCountry, userNotes: draft.userNotes,
      maxContextTokens: draft.maxContextTokens, keepRecentMsgs: draft.keepRecentMsgs,
      recallTopN: draft.recallTopN, recallMinScore: draft.recallMinScore,
      defaultDailyCallLimit: draft.defaultDailyCallLimit, defaultDailyTokenLimit: draft.defaultDailyTokenLimit,
      defaultHeartbeatSec: draft.defaultHeartbeatSec, pauseAutonomy: draft.pauseAutonomy,
      autoTitleEnabled: draft.autoTitleEnabled, titleModel: draft.titleModel,
      mcpGatewayUrl: draft.mcpGatewayUrl, logLevel: draft.logLevel,
    }
    if (keyInput) patch.anthropicKey = keyInput
    if (minimaxKeyInput) patch.minimaxKey = minimaxKeyInput
    const updated = await api.updateSettings(patch)
    setDraft(updated); setOriginal(updated); setKeyInput(''); setMinimaxKeyInput('')
    onSaved(updated)
  }

  const saveWs = async () => {
    if (!ws) return
    const updated = await api.updateWorkspaceSettings({
      // instructions are edited (and saved) in the "Promptlar & Dosyalar" tab as
      // an editable file; omit here so a Genel save never clobbers a newer value.
      name: ws.name, icon: ws.icon, color: ws.color,
      defaultProvider: ws.defaultProvider, defaultModel: ws.defaultModel,
      pauseAutonomy: ws.pauseAutonomy,
    })
    setWs(updated); setWsOrig(updated)
    onWorkspaceChanged?.()
  }

  const save = async () => {
    setSaving(true)
    try {
      if (isWs) await saveWs()
      else await saveApp()
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const clearKey = async (which: 'anthropic' | 'minimax') => {
    try {
      const updated = await api.updateSettings(
        which === 'anthropic' ? { anthropicKey: '' } : { minimaxKey: '' },
      )
      setDraft(updated); setOriginal(updated)
      if (which === 'anthropic') setKeyInput('')
      else setMinimaxKeyInput('')
    } catch (e) {
      onError((e as Error).message)
    }
  }

  const runTest = async (provider: string) => {
    setTest((t) => ({ ...t, [provider]: 'pending' }))
    try {
      const r = await api.testProvider(provider)
      setTest((t) => ({ ...t, [provider]: r }))
    } catch (e) {
      setTest((t) => ({ ...t, [provider]: { ok: false, error: (e as Error).message } }))
    }
  }

  const catLabel = [...APP_CATS, ...WS_CATS].find((c) => c.key === cat)?.label ?? ''

  return (
    <div className="flex h-full">
      {/* Left: category rail */}
      <aside className="flex w-56 flex-shrink-0 flex-col gap-1 overflow-y-auto border-r border-[var(--color-border)] bg-[var(--color-surface)] p-2">
        <div className="px-2 pb-1 pt-2 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
          Uygulama
        </div>
        {APP_CATS.map((c) => (
          <CatButton key={c.key} c={c} active={cat === c.key} onClick={() => setCat(c.key)} dirty={c.key !== 'about' && !!dirtyApp} />
        ))}
        <div className="px-2 pb-1 pt-3 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
          Bu Workspace{ws ? ` · ${ws.name}` : ''}
        </div>
        {WS_CATS.map((c) => (
          <CatButton key={c.key} c={c} active={cat === c.key} onClick={() => setCat(c.key)} dirty={!!dirtyWs} />
        ))}
      </aside>

      {/* Right: content for the active category */}
      <div className="flex flex-1 flex-col">
        <div className="flex items-center justify-between border-b border-[var(--color-border)] px-6 py-3">
          <span className="text-sm font-semibold">{catLabel}</span>
          <div className="flex items-center gap-3">
            <span className="text-xs text-[var(--color-text-dim)]">
              {dirty ? 'Kaydedilmemiş değişiklik' : 'Kayıtlı'}
            </span>
            {cat !== 'about' && cat !== 'commands' && cat !== 'stepkinds' && cat !== 'wsfiles' && (
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

        <div className="mx-auto w-full max-w-2xl flex-1 space-y-4 overflow-y-auto p-6">
          {!draft || !ws ? (
            <div className="text-sm text-[var(--color-text-dim)]">Yükleniyor…</div>
          ) : (
            <>
              {cat === 'profile' && <ProfilePanel draft={draft} set={set} setDraft={setDraft} />}
              {cat === 'notifications' && <NotificationsPanel draft={draft} set={set} setDraft={setDraft} />}
              {cat === 'appearance' && <AppearancePanel draft={draft} set={set} setDraft={setDraft} />}
              {cat === 'providers' && (
                <ProvidersPanel
                  draft={draft}
                  set={set}
                  setDraft={setDraft}
                  keyInput={keyInput}
                  setKeyInput={setKeyInput}
                  minimaxKeyInput={minimaxKeyInput}
                  setMinimaxKeyInput={setMinimaxKeyInput}
                  test={test}
                  runTest={runTest}
                  clearKey={clearKey}
                />
              )}
              {cat === 'context' && <ContextPanel draft={draft} set={set} setDraft={setDraft} />}
              {cat === 'budget' && <BudgetPanel draft={draft} set={set} setDraft={setDraft} />}
              {cat === 'autonomy' && <AutonomyPanel draft={draft} set={set} setDraft={setDraft} />}
              {cat === 'autotitle' && <AutoTitlePanel draft={draft} set={set} setDraft={setDraft} />}
              {cat === 'mcp' && <McpPanel draft={draft} set={set} setDraft={setDraft} />}
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
              {cat === 'diagnostics' && <DiagnosticsPanel draft={draft} set={set} setDraft={setDraft} />}
              {cat === 'about' && <AboutPanel />}
              {cat === 'workspace' && (
                <WorkspacePanel ws={ws} setWsField={setWsField} onDeleteWorkspace={onDeleteWorkspace} />
              )}
              {cat === 'wsfiles' && <WorkspaceFilesPanel onError={onError} />}
            </>
          )}
        </div>
      </div>
    </div>
  )
}
