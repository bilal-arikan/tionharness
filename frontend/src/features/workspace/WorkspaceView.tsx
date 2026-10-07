import { useEffect, useMemo, useState } from 'react'
import {
  Bot,
  Boxes,
  Cpu,
  ScrollText,
  FolderGit2,
  Lightbulb,
  Palette,
  PackageCheck,
  type LucideIcon,
} from 'lucide-react'
import { api } from '@/api'
import type { WorkspaceSettings } from '@/types'
import type { Appearance } from '@/shared/lib/theme'
import { WorkspacePanel } from '@/features/settings/WorkspacePanel'
import { AppearancePanel } from '@/features/settings/appPanels'
import { LogsPanel } from '@/features/logs/LogsPanel'
import { ProjectPanel } from './ProjectPanel'
import { ProcessPanel } from './ProcessPanel'
import { ExportIntro, WorkspaceExportPanel } from './WorkspaceExportPanel'
import { RecommendationsPanel } from './RecommendationsPanel'
import { WorkspaceAgentsPanel } from './WorkspaceAgentsPanel'
import { Button, CollapsibleListShell, InfoPopover, toast } from '@/shared/components'
import { useRegisterDirty } from '@/shared/lib/dirtySignals'
import { useTranslation } from 'react-i18next'

type Tab =
  | 'general'
  | 'agents'
  | 'appearance'
  | 'project'
  | 'logs'
  | 'processes'
  | 'export'
  | 'recommendations'

const WORKSPACE_TAB_KEYS: Tab[] = [
  'general',
  'agents',
  'appearance',
  'project',
  'logs',
  'processes',
  'export',
  'recommendations',
]

// Tabs that render their own PaneHeader and span the full pane: they get no
// workspace header, no Save button and no centered form column.
const FULL_PANE_TABS: Tab[] = ['logs', 'processes']

interface Props {
  onError: (msg: string) => void
  // Refresh the workspace list/switcher after a rename/icon/color change.
  onWorkspaceChanged?: () => void
  // Delete the active workspace (App handles confirm/switch).
  onDeleteWorkspace?: () => void
  // Sync App's live theme after the per-workspace appearance override is saved.
  onAppearanceSaved?: (a: Appearance) => void
  // Re-trigger the post-create recommendation toast on demand (bumps recsTrigger in
  // App) — used by the Öneriler tab's "show cards" button.
  onShowRecommendations?: () => void
  // Open a session's transcript — used by the İşlemler tab's owner column to jump
  // to the session that started a process.
  onOpenSession?: (sessionId: string) => void
  // Active sub-tab, URL-synced by the parent (#/w/{ws}/workspace/{tab}).
  tab?: string | null
  onTabChange?: (t: string) => void
  // Left sub-navbar collapse — controlled from the app header's list toggle.
  navOpen?: boolean
  onToggleNav?: () => void
}

const TABS: { key: Tab; labelKey: string; icon: LucideIcon }[] = [
  { key: 'general', labelKey: 'view.tabs.general', icon: Boxes },
  { key: 'agents', labelKey: 'view.tabs.agents', icon: Bot },
  { key: 'appearance', labelKey: 'view.tabs.appearance', icon: Palette },
  { key: 'project', labelKey: 'view.tabs.project', icon: FolderGit2 },
  { key: 'logs', labelKey: 'view.tabs.logs', icon: ScrollText },
  { key: 'processes', labelKey: 'view.tabs.processes', icon: Cpu },
  { key: 'export', labelKey: 'view.tabs.export', icon: PackageCheck },
  { key: 'recommendations', labelKey: 'view.tabs.recommendations', icon: Lightbulb },
]

// WorkspaceView is the dedicated workspace window opened from the NavRail. A
// left sub-navbar (like the Settings / Logs screens) selects between the active
// workspace's General settings, Project (path + git), and logs.
export function WorkspaceView({
  onError,
  onWorkspaceChanged,
  onDeleteWorkspace,
  onAppearanceSaved,
  onShowRecommendations,
  onOpenSession,
  tab: tabProp,
  onTabChange,
  navOpen,
  onToggleNav,
}: Props) {
  const { t } = useTranslation('workspace')
  const tab: Tab = WORKSPACE_TAB_KEYS.includes(tabProp as Tab) ? (tabProp as Tab) : 'general'
  const setTab = (t: Tab) => onTabChange?.(t)
  const [ws, setWs] = useState<WorkspaceSettings | null>(null)
  const [wsOrig, setWsOrig] = useState<WorkspaceSettings | null>(null)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    api
      .getWorkspaceSettings()
      .then((s) => {
        setWs(s)
        setWsOrig(s)
      })
      .catch((e) => onError((e as Error).message))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const dirty = useMemo(
    () => !!ws && !!wsOrig && JSON.stringify(ws) !== JSON.stringify(wsOrig),
    [ws, wsOrig],
  )

  const setWsField = <K extends keyof WorkspaceSettings>(key: K, val: WorkspaceSettings[K]) =>
    setWs((d) => (d ? { ...d, [key]: val } : d))

  const save = async () => {
    if (!ws) return
    setSaving(true)
    try {
      const updated = await api.updateWorkspaceSettings({
        // instructions are edited (and saved) in the Files tab as an editable
        // file; omit here so a save never clobbers a newer value.
        name: ws.name,
        icon: ws.icon,
        color: ws.color,
        pauseAutonomy: ws.pauseAutonomy,
        defaultWorkingDir: ws.defaultWorkingDir,
        terseMode: ws.terseMode,
        codebaseMemoryEnabled: ws.codebaseMemoryEnabled,
        zvecGrepEnabled: ws.zvecGrepEnabled,
        promptEpochEnabled: ws.promptEpochEnabled,
        shellOutputCompression: ws.shellOutputCompression,
        shellCommandRewrite: ws.shellCommandRewrite,
        // The awareness block is replaced as a whole (the backend normalizes it).
        awareness: ws.awareness,
      })
      setWs(updated)
      setWsOrig(updated)
      onWorkspaceChanged?.()
      toast.success(t('view.savedToast'))
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const headerDirty = dirty
  const headerSaving = saving
  const onHeaderSave = save
  // Appearance/Export/Recommendations manage their own actions. Logs and
  // İşlemler render their own PaneHeader, so the workspace header is omitted for
  // those tabs.
  const fullPane = FULL_PANE_TABS.includes(tab)
  const selfSaving: Tab[] = ['agents', 'appearance', 'export', 'recommendations']
  const showSave = selfSaving.includes(tab) ? false : !fullPane
  const activeMeta = TABS.find((t) => t.key === tab)

  // Surface unsaved workspace edits on the nav "Workspace" item + workspace label.
  useRegisterDirty('workspace', headerDirty)

  return (
    <div className="flex min-h-0 flex-1">
      {/* Left sub-navbar (collapsible via the app header toggle; mobile drawer) */}
      <CollapsibleListShell
        open={navOpen ?? true}
        onToggle={onToggleNav ?? (() => {})}
        label={t('view.title')}
      >
        <aside className="th-col flex h-full w-56 flex-shrink-0 flex-col gap-1 overflow-y-auto border-r border-[var(--color-border)] bg-[var(--color-surface)] p-2 max-md:w-[85vw] max-md:max-w-sm square:w-48">
          <div className="px-2 pb-1 pt-2 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
            {t('view.title')}
            {ws ? ` · ${ws.name}` : ''}
          </div>
          {TABS.map((tabMeta) => (
            <button
              key={tabMeta.key}
              onClick={() => setTab(tabMeta.key)}
              className={`flex items-center gap-2 rounded-lg px-3 py-2 text-left text-sm transition ${
                tab === tabMeta.key
                  ? 'bg-[var(--color-accent-soft)] text-[var(--color-text)]'
                  : 'text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)]'
              }`}
            >
              <tabMeta.icon size={16} className="shrink-0" />
              <span className="flex-1 truncate">{t(tabMeta.labelKey)}</span>
              {(FULL_PANE_TABS.includes(tabMeta.key)
                ? false
                : selfSaving.includes(tabMeta.key)
                  ? false
                  : dirty) && (
                <span
                  className="h-1.5 w-1.5 rounded-full bg-[var(--color-accent)]"
                  title={t('view.unsaved')}
                />
              )}
            </button>
          ))}
        </aside>
      </CollapsibleListShell>

      {/* Right content */}
      {/* min-w-0: without it this flex-1 column can't shrink below its content's
          intrinsic width, so a wide prompt preview (code blocks/tables/long lines
          in the Files tab) pushes the whole column past the viewport. */}
      <div className="flex min-w-0 flex-1 flex-col">
        {!fullPane && (
          <div className="flex items-center justify-between border-b border-[var(--color-border)] px-6 py-3">
            <span className="flex items-center gap-2 text-sm font-semibold">
              {activeMeta && (
                <span className="flex h-6 w-6 items-center justify-center rounded-md bg-[var(--color-accent-soft)] text-[var(--color-accent)]">
                  <activeMeta.icon size={14} />
                </span>
              )}
              {activeMeta ? t(activeMeta.labelKey) : ''}
              {/* Tab-level explanations live behind an (ⓘ) next to the title. */}
              {tab === 'export' && ws && <InfoPopover text={<ExportIntro name={ws.name} />} />}
              {tab === 'recommendations' && (
                <InfoPopover text={t('recommendations.panel.description')} />
              )}
            </span>
            {showSave && (
              <div className="flex items-center gap-3">
                <span className="text-xs text-[var(--color-text-dim)]">
                  {headerDirty ? t('view.unsavedChanges') : t('view.saved')}
                </span>
                <Button onClick={onHeaderSave} disabled={!headerDirty || headerSaving}>
                  {headerSaving ? t('actions.saving') : t('actions.save')}
                </Button>
              </div>
            )}
          </div>
        )}

        {/* Logs/İşlemler span the full pane; form tabs stay in a centered column. */}
        <div
          className={`mx-auto w-full min-w-0 flex-1 ${
            fullPane
              ? 'flex max-w-none overflow-hidden'
              : 'th-column max-w-2xl space-y-4 overflow-y-auto p-4 sm:p-6 3xl:max-w-4xl'
          }`}
        >
          {!ws ? (
            <div className="text-sm text-[var(--color-text-dim)]">{t('actions.loading')}</div>
          ) : tab === 'general' ? (
            <WorkspacePanel ws={ws} setWsField={setWsField} onDeleteWorkspace={onDeleteWorkspace} />
          ) : tab === 'agents' ? (
            <WorkspaceAgentsPanel />
          ) : tab === 'appearance' ? (
            <AppearancePanel onError={onError} onAppearanceSaved={onAppearanceSaved} />
          ) : tab === 'project' ? (
            <ProjectPanel
              path={ws.defaultWorkingDir ?? ''}
              onSelectPath={(p) => setWsField('defaultWorkingDir', p)}
              onError={onError}
            />
          ) : tab === 'export' ? (
            <WorkspaceExportPanel ws={ws} onError={onError} />
          ) : tab === 'recommendations' ? (
            <RecommendationsPanel onError={onError} onShowCards={onShowRecommendations} />
          ) : tab === 'logs' ? (
            <LogsPanel onError={onError} />
          ) : tab === 'processes' ? (
            <ProcessPanel onError={onError} onOpenSession={onOpenSession} />
          ) : (
            <div />
          )}
        </div>
      </div>
    </div>
  )
}
