import { useEffect, useState } from 'react'
import {
  MessageSquare,
  Activity,
  Users,
  Share2,
  LayoutGrid,
  Clock,
  Database,
  GitBranch,
  FileCode,
  ScrollText,
  Sparkles,
  Store,
  Wallet,
  Boxes,
  Plug,
  Settings,
  ChevronLeft,
  type LucideIcon,
} from 'lucide-react'
import type { Workspace } from '../types'
import { WorkspaceSwitcher } from './workspace/WorkspaceSwitcher'
import type { NewWorkspaceData } from './workspace/WorkspaceCreateModal'

export type View = 'chat' | 'executions' | 'agents' | 'network' | 'board' | 'schedules' | 'memory' | 'flows' | 'artifacts' | 'skills' | 'tools' | 'market' | 'budget' | 'logs' | 'workspace' | 'settings'

interface Props {
  view: View
  onSelectView: (v: View) => void
  workspaces: Workspace[]
  activeWorkspaceId: string | null
  unreadWorkspaceIds: Set<string>
  // Per-view notification signals, each shown with a distinct dot on the nav item:
  //   busy   → pulsing accent dot (work running)
  //   unread → solid accent dot (unseen activity)
  //   dirty  → amber dot (unsaved local edits)
  // The sets are scoped to the active workspace; their union also rolls up onto
  // the workspace label so a glance shows where attention is needed.
  busyViews?: Set<View>
  unreadViews?: Set<View>
  dirtyViews?: Set<View>
  // The workspace that opens on a fresh launch (toggled via the switcher star).
  favoriteWorkspaceId?: string | null
  onSetFavoriteWorkspace?: (id: string) => void
  onSwitchWorkspace: (id: string) => void
  onCreateWorkspace: (data: NewWorkspaceData) => void
  onDeleteWorkspace: (id: string) => void
}

// NAV is the primary view list. Exported so the mobile bottom bar renders the
// same set from a single source of truth.
export const NAV: { key: View; label: string; icon: LucideIcon }[] = [
  { key: 'chat', label: 'Sohbet', icon: MessageSquare },
  { key: 'executions', label: 'Aktivite', icon: Activity },
  { key: 'agents', label: 'Ajanlar', icon: Users },
  { key: 'network', label: 'Ağ', icon: Share2 },
  { key: 'board', label: 'Görevler', icon: LayoutGrid },
  { key: 'schedules', label: 'Otomasyon', icon: Clock },
  { key: 'memory', label: 'Hafıza', icon: Database },
  { key: 'flows', label: 'Akışlar', icon: GitBranch },
  { key: 'artifacts', label: 'Artifactlar', icon: FileCode },
  { key: 'skills', label: 'Skills', icon: Sparkles },
  { key: 'tools', label: 'Araçlar & MCP', icon: Plug },
  { key: 'market', label: 'Market', icon: Store },
  { key: 'budget', label: 'Bütçe', icon: Wallet },
  { key: 'logs', label: 'Loglar', icon: ScrollText },
]

const COLLAPSE_KEY = 'swarmgo.navCollapsed'

// navItemClass renders the shared look for a nav button. The active state is a
// soft accent tint with an accent left indicator (instead of a heavy solid
// fill), which reads as more modern and less visually loud.
function navItemClass(active: boolean, collapsed: boolean): string {
  return [
    'group relative flex items-center gap-3 rounded-lg px-3 py-2 text-sm transition',
    collapsed ? 'justify-center' : '',
    active
      ? 'bg-[var(--color-accent-soft)] font-medium text-[var(--color-accent)]'
      : 'text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]',
  ].join(' ')
}

// ActiveBar is the 2px accent indicator on the left edge of the active item.
function ActiveBar() {
  return (
    <span className="absolute left-0 top-1/2 h-5 w-[3px] -translate-y-1/2 rounded-full bg-[var(--color-accent)]" />
  )
}

// NavDots renders the per-item notification cluster: an amber dot for unsaved
// edits (dirty) plus an accent dot for activity — pulsing when busy, solid when
// merely unread. Collapsed rail uses corner dots; expanded uses a right cluster.
function NavDots({
  busy,
  unread,
  dirty,
  collapsed,
}: {
  busy?: boolean
  unread?: boolean
  dirty?: boolean
  collapsed?: boolean
}) {
  if (!busy && !unread && !dirty) return null
  const accentTitle = busy ? 'İşlem sürüyor' : 'Yeni etkinlik'
  if (collapsed) {
    return (
      <>
        {dirty && (
          <span
            className="absolute left-1 top-1 h-2 w-2 rounded-full bg-[var(--color-warning)] ring-2 ring-[var(--color-surface)]"
            title="Kaydedilmemiş değişiklik"
          />
        )}
        {(busy || unread) && (
          <span
            className={`absolute right-1 top-1 h-2 w-2 rounded-full bg-[var(--color-accent)] ring-2 ring-[var(--color-surface)] ${
              busy ? 'animate-pulse' : ''
            }`}
            title={accentTitle}
          />
        )}
      </>
    )
  }
  return (
    <span className="ml-auto flex items-center gap-1.5">
      {dirty && (
        <span className="h-2 w-2 rounded-full bg-[var(--color-warning)]" title="Kaydedilmemiş değişiklik" />
      )}
      {(busy || unread) && (
        <span
          className={`h-2 w-2 rounded-full bg-[var(--color-accent)] ${busy ? 'animate-pulse' : ''}`}
          title={accentTitle}
        />
      )}
    </span>
  )
}

// NavRail is the leftmost column: brand, workspace switcher, and the primary
// view navigation. It collapses to an icon-only rail to maximise content space.
export function NavRail({
  view,
  onSelectView,
  workspaces,
  activeWorkspaceId,
  unreadWorkspaceIds,
  busyViews,
  unreadViews,
  dirtyViews,
  favoriteWorkspaceId,
  onSetFavoriteWorkspace,
  onSwitchWorkspace,
  onCreateWorkspace,
  onDeleteWorkspace,
}: Props) {
  // The active workspace's signals rolled up for its label: any busy view, any
  // unsaved edit. (Other workspaces surface via the unread-badge set.)
  const anyBusy = (busyViews?.size ?? 0) > 0
  const anyDirty = (dirtyViews?.size ?? 0) > 0
  const [collapsed, setCollapsed] = useState(
    () => localStorage.getItem(COLLAPSE_KEY) === '1',
  )

  useEffect(() => {
    localStorage.setItem(COLLAPSE_KEY, collapsed ? '1' : '0')
  }, [collapsed])

  const active = workspaces.find((w) => w.id === activeWorkspaceId)

  return (
    <nav
      className={`hidden h-full flex-col border-r border-[var(--color-border)] bg-[var(--color-surface)] shadow-[var(--shadow-sm)] transition-all duration-200 md:flex ${
        collapsed ? 'w-14' : 'w-52'
      }`}
    >
      {/* Brand */}
      <div className="flex h-14 items-center gap-2 px-3">
        <div className="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-lg bg-gradient-to-br from-[var(--color-accent)] to-[color-mix(in_srgb,var(--color-accent)_60%,#000)] text-sm font-bold text-white shadow-[var(--shadow-sm)]">
          SG
        </div>
        {!collapsed && <span className="text-lg font-semibold tracking-tight">SwarmGo</span>}
      </div>

      {/* Workspace */}
      {collapsed ? (
        <button
          onClick={() => setCollapsed(false)}
          title={active?.name ?? 'Workspace seç'}
          className="relative mx-2 mb-2 flex h-9 items-center justify-center rounded-lg bg-[var(--color-surface-2)] text-sm font-medium hover:opacity-90"
          style={active?.color ? { backgroundColor: active.color + '33' } : undefined}
        >
          {active?.icon || (active?.name ?? '?').charAt(0).toUpperCase()}
          {unreadWorkspaceIds.size > 0 && (
            <span className="absolute right-1 top-1 h-2 w-2 rounded-full bg-[var(--color-accent)] ring-2 ring-[var(--color-surface-2)]" />
          )}
          {anyDirty && (
            <span
              className="absolute left-1 top-1 h-2 w-2 rounded-full bg-[var(--color-warning)] ring-2 ring-[var(--color-surface-2)]"
              title="Kaydedilmemiş değişiklik"
            />
          )}
          {anyBusy && (
            <span
              className="absolute bottom-1 right-1 h-2 w-2 animate-pulse rounded-full bg-[var(--color-accent)] ring-2 ring-[var(--color-surface-2)]"
              title="İşlem sürüyor"
            />
          )}
        </button>
      ) : (
        <WorkspaceSwitcher
          workspaces={workspaces}
          activeId={activeWorkspaceId}
          unreadIds={unreadWorkspaceIds}
          activeBusy={anyBusy}
          activeDirty={anyDirty}
          favoriteId={favoriteWorkspaceId}
          onToggleFavorite={onSetFavoriteWorkspace}
          onSwitch={onSwitchWorkspace}
          onCreate={onCreateWorkspace}
          onDelete={onDeleteWorkspace}
          trailing={
            <button
              onClick={() => setCollapsed(true)}
              title="Daralt"
              aria-label="Navbarı daralt"
              className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg text-[var(--color-text-dim)] transition hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
            >
              <ChevronLeft size={18} />
            </button>
          }
        />
      )}

      {/* View navigation */}
      <div className="flex flex-1 flex-col gap-1 px-2 py-2">
        {NAV.map((item) => {
          const Icon = item.icon
          const isActive = view === item.key
          const busy = busyViews?.has(item.key) ?? false
          const unread = unreadViews?.has(item.key) ?? false
          const dirty = dirtyViews?.has(item.key) ?? false
          return (
            <button
              key={item.key}
              onClick={() => onSelectView(item.key)}
              data-testid={`nav-${item.key}`}
              aria-label={item.label}
              aria-current={isActive ? 'page' : undefined}
              title={collapsed ? `${item.label}${busy ? ' · işlem sürüyor' : unread ? ' · yeni etkinlik' : ''}` : undefined}
              className={navItemClass(isActive, collapsed)}
            >
              {isActive && <ActiveBar />}
              <Icon size={18} strokeWidth={2} className="shrink-0" />
              {!collapsed && <span>{item.label}</span>}
              <NavDots busy={busy} unread={unread} dirty={dirty} collapsed={collapsed} />
            </button>
          )
        })}
      </div>

      {/* Workspace + Settings (pinned at the bottom, separate from primary nav) */}
      <div className="flex flex-col gap-1 px-2 pb-1">
        <button
          onClick={() => onSelectView('workspace')}
          data-testid="nav-workspace"
          aria-label="Workspace"
          aria-current={view === 'workspace' ? 'page' : undefined}
          title={collapsed ? (active?.name ? `Workspace · ${active.name}` : 'Workspace') : undefined}
          className={`w-full ${navItemClass(view === 'workspace', collapsed)}`}
        >
          {view === 'workspace' && <ActiveBar />}
          <Boxes size={18} strokeWidth={2} className="shrink-0" />
          {!collapsed && <span>Workspace</span>}
          <NavDots dirty={dirtyViews?.has('workspace')} collapsed={collapsed} />
        </button>
        <button
          onClick={() => onSelectView('settings')}
          data-testid="nav-settings"
          aria-label="Ayarlar"
          aria-current={view === 'settings' ? 'page' : undefined}
          title={collapsed ? 'Ayarlar' : undefined}
          className={`w-full ${navItemClass(view === 'settings', collapsed)}`}
        >
          {view === 'settings' && <ActiveBar />}
          <Settings size={18} strokeWidth={2} className="shrink-0" />
          {!collapsed && <span>Ayarlar</span>}
          <NavDots dirty={dirtyViews?.has('settings')} collapsed={collapsed} />
        </button>
      </div>
    </nav>
  )
}
