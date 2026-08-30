import { useEffect, useMemo, useState } from 'react'
import {
  Boxes,
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
import { WorkspaceExportPanel } from './WorkspaceExportPanel'
import { RecommendationsPanel } from './RecommendationsPanel'
import { Button, CollapsibleListShell, toast } from '@/shared/components'
import { useRegisterDirty } from '@/shared/lib/dirtySignals'
import { useTranslation } from 'react-i18next'

type Tab = 'general' | 'appearance' | 'project' | 'logs' | 'export' | 'recommendations'

// eslint-disable-next-line react-refresh/only-export-components
export const WORKSPACE_TAB_KEYS: Tab[] = [
  'general',
  'appearance',
  'project',
  'logs',
  'export',
  'recommendations',
]

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
  // Active sub-tab, URL-synced by the parent (#/w/{ws}/workspace/{tab}).
  tab?: string | null
  onTabChange?: (t: string) => void
  // Left sub-navbar collapse — controlled from the app header's list toggle.
  navOpen?: boolean
  onToggleNav?: () => void
}

const TABS: { key: Tab; label: string; labelKey?: string; icon: LucideIcon }[] = [
  { key: 'general', label: 'Genel', icon: Boxes },
  { key: 'appearance', label: 'Görünüm', icon: Palette },
  { key: 'project', label: 'Proje', icon: FolderGit2 },
  { key: 'logs', label: 'Logs', labelKey: 'workspace.tabs.logs', icon: ScrollText },
  { key: 'export', label: 'Dışa Aktar', icon: PackageCheck },
  { key: 'recommendations', label: 'Öneriler', icon: Lightbulb },
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
  tab: tabProp,
  onTabChange,
  navOpen,
  onToggleNav,
}: Props) {
  const { t: translate } = useTranslation()
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
        promptEpochEnabled: ws.promptEpochEnabled,
        shellOutputCompression: ws.shellOutputCompression,
        shellCommandRewrite: ws.shellCommandRewrite,
      })
      setWs(updated)
      setWsOrig(updated)
      onWorkspaceChanged?.()
      toast.success('Kaydedildi')
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const headerDirty = dirty
  const headerSaving = saving
  const onHeaderSave = save
  // Appearance/Export/Recommendations manage their own actions. Logs renders its
  // own PaneHeader, so the workspace header is omitted for that tab.
  const showSave =
    tab === 'appearance' || tab === 'export' || tab === 'recommendations' ? false : tab !== 'logs'
  const activeMeta = TABS.find((t) => t.key === tab)

  // Surface unsaved workspace edits on the nav "Workspace" item + workspace label.
  useRegisterDirty('workspace', headerDirty)

  return (
    <div className="flex min-h-0 flex-1">
      {/* Left sub-navbar (collapsible via the app header toggle; mobile drawer) */}
      <CollapsibleListShell
        open={navOpen ?? true}
        onToggle={onToggleNav ?? (() => {})}
        label="Workspace"
        hideRail
      >
        <aside className="flex h-full w-56 flex-shrink-0 flex-col gap-1 overflow-y-auto border-r border-[var(--color-border)] bg-[var(--color-surface)] p-2 max-md:w-[85vw] max-md:max-w-sm">
          <div className="px-2 pb-1 pt-2 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
            Workspace{ws ? ` · ${ws.name}` : ''}
          </div>
          {TABS.map((t) => (
            <button
              key={t.key}
              onClick={() => setTab(t.key)}
              className={`flex items-center gap-2 rounded-lg px-3 py-2 text-left text-sm transition ${
                tab === t.key
                  ? 'bg-[var(--color-accent-soft)] text-[var(--color-text)]'
                  : 'text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)]'
              }`}
            >
              <t.icon size={16} className="shrink-0" />
              <span className="flex-1 truncate">
                {t.labelKey ? translate(t.labelKey) : t.label}
              </span>
              {(t.key === 'logs'
                ? false
                : t.key === 'appearance' || t.key === 'export' || t.key === 'recommendations'
                  ? false
                  : dirty) && (
                <span
                  className="h-1.5 w-1.5 rounded-full bg-[var(--color-accent)]"
                  title="Kaydedilmemiş"
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
        {tab !== 'logs' && (
          <div className="flex items-center justify-between border-b border-[var(--color-border)] px-6 py-3">
            <span className="flex items-center gap-2 text-sm font-semibold">
              {activeMeta && (
                <span className="flex h-6 w-6 items-center justify-center rounded-md bg-[var(--color-accent-soft)] text-[var(--color-accent)]">
                  <activeMeta.icon size={14} />
                </span>
              )}
              {activeMeta
                ? activeMeta.labelKey
                  ? translate(activeMeta.labelKey)
                  : activeMeta.label
                : ''}
            </span>
            {showSave && (
              <div className="flex items-center gap-3">
                <span className="text-xs text-[var(--color-text-dim)]">
                  {headerDirty ? 'Kaydedilmemiş değişiklik' : 'Kayıtlı'}
                </span>
                <Button onClick={onHeaderSave} disabled={!headerDirty || headerSaving}>
                  {headerSaving ? 'Kaydediliyor…' : 'Kaydet'}
                </Button>
              </div>
            )}
          </div>
        )}

        {/* Logs spans the full pane; form tabs stay in a centered column. */}
        <div
          className={`mx-auto w-full min-w-0 flex-1 ${
            tab === 'logs'
              ? 'flex max-w-none overflow-hidden'
              : 'max-w-2xl space-y-4 overflow-y-auto p-6'
          }`}
        >
          {!ws ? (
            <div className="text-sm text-[var(--color-text-dim)]">Yükleniyor…</div>
          ) : tab === 'general' ? (
            <WorkspacePanel ws={ws} setWsField={setWsField} onDeleteWorkspace={onDeleteWorkspace} />
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
          ) : (
            <div />
          )}
        </div>
      </div>
    </div>
  )
}
