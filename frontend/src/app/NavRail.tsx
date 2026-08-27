import { useEffect, useState } from 'react'
import { Settings, ChevronLeft, Boxes } from 'lucide-react'
import type { Workspace } from '@/types'
import { WorkspaceSwitcher } from '@/features/workspace/WorkspaceSwitcher'
import type { NewWorkspaceData } from '@/features/workspace/WorkspaceCreateModal'
import { NAV } from './navItems'

export type View =
  | 'dashboard'
  | 'chat'
  | 'agents'
  | 'network'
  | 'explorer'
  | 'board'
  | 'schedules'
  | 'flows'
  | 'artifacts'
  | 'skills'
  | 'tools'
  | 'market'
  | 'budget'
  | 'logs'
  | 'insights'
  | 'workspace'
  | 'settings'

interface Props {
  view: View
  onSelectView: (v: View) => void
  workspaces: Workspace[]
  activeWorkspaceId: string | null
  unreadWorkspaceIds: Set<string>
  // Workspaces (active OR not) with a run currently in flight, from
  // GET /api/workspaces/activity. Drives the switcher's per-row "çalışıyor" pulse
  // and — when a NON-active workspace is busy — a pulsing rollup on the rail label.
  busyWorkspaceIds?: Set<string>
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
}

// NAV is the primary view list. Exported so the mobile bottom bar renders the
// same set from a single source of truth.
const COLLAPSE_KEY = 'tionharness.navCollapsed'

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
        <span
          className="h-2 w-2 rounded-full bg-[var(--color-warning)]"
          title="Kaydedilmemiş değişiklik"
        />
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
  busyWorkspaceIds,
  busyViews,
  unreadViews,
  dirtyViews,
  favoriteWorkspaceId,
  onSetFavoriteWorkspace,
  onSwitchWorkspace,
  onCreateWorkspace,
}: Props) {
  // The active workspace's signals rolled up for its label: any busy view, any
  // unsaved edit. (Other workspaces surface via the unread-badge set.)
  const anyBusy = (busyViews?.size ?? 0) > 0
  const anyDirty = (dirtyViews?.size ?? 0) > 0
  // A run is in flight in some OTHER workspace (not the one on screen). Turns the
  // "other workspace has activity" unread dot into a pulsing one so a live run
  // elsewhere reads differently from a merely-unseen completed one.
  const anyOtherBusy = Array.from(busyWorkspaceIds ?? []).some((id) => id !== activeWorkspaceId)
  const [collapsed, setCollapsed] = useState(() => localStorage.getItem(COLLAPSE_KEY) === '1')

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
        <div className="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-lg bg-[var(--color-accent)] text-sm font-bold text-[var(--color-on-accent)] shadow-[var(--shadow-sm)]">
          TS
        </div>
        {!collapsed && <span className="text-lg font-semibold tracking-tight">TionHarness</span>}
      </div>

      {/* Workspace */}
      {collapsed ? (
        <button
          onClick={() => setCollapsed(false)}
          title={active?.name ?? 'Workspace seç'}
          className="relative mx-2 mb-2 flex h-9 items-center justify-center rounded-lg bg-[var(--color-surface-2)] text-sm font-medium hover:opacity-90"
          style={
            active?.color
              ? { backgroundColor: `color-mix(in srgb, ${active.color} 20%, transparent)` }
              : undefined
          }
        >
          {active?.icon || (active?.name ?? '?').charAt(0).toUpperCase()}
          {(unreadWorkspaceIds.size > 0 || anyOtherBusy) && (
            <span
              className={`absolute right-1 top-1 h-2 w-2 rounded-full bg-[var(--color-accent)] ring-2 ring-[var(--color-surface-2)] ${
                anyOtherBusy ? 'animate-pulse' : ''
              }`}
              title={
                anyOtherBusy
                  ? 'Başka workspace’te işlem sürüyor'
                  : 'Başka workspace’te yeni etkinlik'
              }
            />
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
          busyIds={busyWorkspaceIds}
          activeBusy={anyBusy}
          activeDirty={anyDirty}
          favoriteId={favoriteWorkspaceId}
          onToggleFavorite={onSetFavoriteWorkspace}
          onSwitch={onSwitchWorkspace}
          onCreate={onCreateWorkspace}
          onOpenSettings={(id) => {
            onSwitchWorkspace(id)
            onSelectView('workspace')
          }}
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
      <div className="flex min-h-0 flex-1 flex-col gap-1 overflow-y-auto px-2 py-2">
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
              title={
                collapsed
                  ? `${item.label}${busy ? ' · işlem sürüyor' : unread ? ' · yeni etkinlik' : ''}`
                  : undefined
              }
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
      <div className="flex shrink-0 flex-col gap-1 px-2 pb-1">
        <button
          onClick={() => onSelectView('workspace')}
          data-testid="nav-workspace"
          aria-label="Workspace"
          aria-current={view === 'workspace' ? 'page' : undefined}
          title={
            collapsed ? (active?.name ? `Workspace · ${active.name}` : 'Workspace') : undefined
          }
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
