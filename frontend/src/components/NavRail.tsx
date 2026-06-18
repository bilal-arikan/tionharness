import { useEffect, useState } from 'react'
import {
  MessageSquare,
  Activity,
  Users,
  LayoutGrid,
  Clock,
  Database,
  Plug,
  GitBranch,
  FileCode,
  KeyRound,
  ScrollText,
  Sparkles,
  Wallet,
  Settings,
  ChevronLeft,
  ChevronRight,
  type LucideIcon,
} from 'lucide-react'
import type { Workspace } from '../types'
import { WorkspaceSwitcher } from './workspace/WorkspaceSwitcher'
import type { NewWorkspaceData } from './workspace/WorkspaceCreateModal'

export type View = 'chat' | 'executions' | 'agents' | 'board' | 'schedules' | 'memory' | 'tools' | 'flows' | 'artifacts' | 'secrets' | 'skills' | 'budget' | 'logs' | 'settings'

interface Props {
  view: View
  onSelectView: (v: View) => void
  workspaces: Workspace[]
  activeWorkspaceId: string | null
  unreadWorkspaceIds: Set<string>
  // Views with work currently in progress (chat/board/schedules/flows) — shown
  // with a pulsing accent indicator on the nav item.
  busyViews?: Set<View>
  onSwitchWorkspace: (id: string) => void
  onCreateWorkspace: (data: NewWorkspaceData) => void
  onDeleteWorkspace: (id: string) => void
}

const NAV: { key: View; label: string; icon: LucideIcon }[] = [
  { key: 'chat', label: 'Sohbet', icon: MessageSquare },
  { key: 'executions', label: 'Aktivite', icon: Activity },
  { key: 'agents', label: 'Ajanlar', icon: Users },
  { key: 'board', label: 'Görevler', icon: LayoutGrid },
  { key: 'schedules', label: 'Zamanlamalar', icon: Clock },
  { key: 'memory', label: 'Hafıza', icon: Database },
  { key: 'tools', label: 'Araçlar', icon: Plug },
  { key: 'flows', label: 'Akışlar', icon: GitBranch },
  { key: 'artifacts', label: 'Artifactlar', icon: FileCode },
  { key: 'secrets', label: 'Sırlar', icon: KeyRound },
  { key: 'skills', label: 'Beceriler', icon: Sparkles },
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

// NavRail is the leftmost column: brand, workspace switcher, and the primary
// view navigation. It collapses to an icon-only rail to maximise content space.
export function NavRail({
  view,
  onSelectView,
  workspaces,
  activeWorkspaceId,
  unreadWorkspaceIds,
  busyViews,
  onSwitchWorkspace,
  onCreateWorkspace,
  onDeleteWorkspace,
}: Props) {
  const [collapsed, setCollapsed] = useState(
    () => localStorage.getItem(COLLAPSE_KEY) === '1',
  )

  useEffect(() => {
    localStorage.setItem(COLLAPSE_KEY, collapsed ? '1' : '0')
  }, [collapsed])

  const active = workspaces.find((w) => w.id === activeWorkspaceId)

  return (
    <nav
      className={`flex h-full flex-col border-r border-[var(--color-border)] bg-[var(--color-surface)] shadow-[var(--shadow-sm)] transition-all duration-200 ${
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
        </button>
      ) : (
        <WorkspaceSwitcher
          workspaces={workspaces}
          activeId={activeWorkspaceId}
          unreadIds={unreadWorkspaceIds}
          onSwitch={onSwitchWorkspace}
          onCreate={onCreateWorkspace}
          onDelete={onDeleteWorkspace}
        />
      )}

      {/* View navigation */}
      <div className="flex flex-1 flex-col gap-1 px-2 py-2">
        {NAV.map((item) => {
          const Icon = item.icon
          const isActive = view === item.key
          const busy = busyViews?.has(item.key) ?? false
          return (
            <button
              key={item.key}
              onClick={() => onSelectView(item.key)}
              title={collapsed ? `${item.label}${busy ? ' · işlem sürüyor' : ''}` : undefined}
              className={navItemClass(isActive, collapsed)}
            >
              {isActive && <ActiveBar />}
              <Icon size={18} strokeWidth={2} className="shrink-0" />
              {!collapsed && <span>{item.label}</span>}
              {busy &&
                (collapsed ? (
                  <span className="absolute right-1 top-1 h-2 w-2 animate-pulse rounded-full bg-[var(--color-accent)] ring-2 ring-[var(--color-surface)]" />
                ) : (
                  <span
                    className="ml-auto h-2 w-2 animate-pulse rounded-full bg-[var(--color-accent)]"
                    title="İşlem sürüyor"
                  />
                ))}
            </button>
          )
        })}
      </div>

      {/* Settings (pinned at the bottom, separate from primary nav) */}
      <div className="px-2 pb-1">
        <button
          onClick={() => onSelectView('settings')}
          title={collapsed ? 'Ayarlar' : undefined}
          className={`w-full ${navItemClass(view === 'settings', collapsed)}`}
        >
          {view === 'settings' && <ActiveBar />}
          <Settings size={18} strokeWidth={2} className="shrink-0" />
          {!collapsed && <span>Ayarlar</span>}
        </button>
      </div>

      {/* Collapse toggle */}
      <button
        onClick={() => setCollapsed((v) => !v)}
        title={collapsed ? 'Genişlet' : 'Daralt'}
        className="m-2 flex items-center justify-center rounded-lg py-2 text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
      >
        {collapsed ? <ChevronRight size={18} /> : <ChevronLeft size={18} />}
      </button>
    </nav>
  )
}
