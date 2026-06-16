import { useEffect, useState } from 'react'
import type { Workspace } from '../types'
import { WorkspaceSwitcher } from './WorkspaceSwitcher'

export type View = 'chat' | 'board' | 'schedules' | 'memory' | 'tools' | 'flows' | 'logs' | 'settings'

interface Props {
  view: View
  onSelectView: (v: View) => void
  workspaces: Workspace[]
  activeWorkspaceId: string | null
  onSwitchWorkspace: (id: string) => void
  onCreateWorkspace: (name: string) => void
}

const NAV: { key: View; label: string; icon: string }[] = [
  { key: 'chat', label: 'Sohbet', icon: '💬' },
  { key: 'board', label: 'Görevler', icon: '🗂' },
  { key: 'schedules', label: 'Zamanlamalar', icon: '⏰' },
  { key: 'memory', label: 'Hafıza', icon: '⛁' },
  { key: 'tools', label: 'Araçlar', icon: '🔌' },
  { key: 'flows', label: 'Akışlar', icon: '🔀' },
  { key: 'logs', label: 'Loglar', icon: '📜' },
]

const COLLAPSE_KEY = 'swarmgo.navCollapsed'

// NavRail is the leftmost column: brand, workspace switcher, and the primary
// view navigation. It collapses to an icon-only rail to maximise content space.
export function NavRail({
  view,
  onSelectView,
  workspaces,
  activeWorkspaceId,
  onSwitchWorkspace,
  onCreateWorkspace,
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
      className={`flex h-full flex-col border-r border-[var(--color-border)] bg-[var(--color-surface)] transition-all duration-200 ${
        collapsed ? 'w-14' : 'w-52'
      }`}
    >
      {/* Brand */}
      <div className="flex h-14 items-center gap-2 px-3">
        <div className="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-lg bg-[var(--color-accent)] text-sm font-bold text-white">
          SG
        </div>
        {!collapsed && <span className="text-lg font-semibold">SwarmGo</span>}
      </div>

      {/* Workspace */}
      {collapsed ? (
        <button
          onClick={() => setCollapsed(false)}
          title={active?.name ?? 'Workspace seç'}
          className="mx-2 mb-2 flex h-9 items-center justify-center rounded-lg bg-[var(--color-surface-2)] text-sm font-medium hover:opacity-90"
          style={active?.color ? { backgroundColor: active.color + '33' } : undefined}
        >
          {active?.icon || (active?.name ?? '?').charAt(0).toUpperCase()}
        </button>
      ) : (
        <WorkspaceSwitcher
          workspaces={workspaces}
          activeId={activeWorkspaceId}
          onSwitch={onSwitchWorkspace}
          onCreate={onCreateWorkspace}
        />
      )}

      {/* View navigation */}
      <div className="flex flex-1 flex-col gap-1 px-2 py-2">
        {NAV.map((item) => (
          <button
            key={item.key}
            onClick={() => onSelectView(item.key)}
            title={collapsed ? item.label : undefined}
            className={`flex items-center gap-3 rounded-lg px-3 py-2 text-sm transition ${
              collapsed ? 'justify-center' : ''
            } ${
              view === item.key
                ? 'bg-[var(--color-accent)] text-white'
                : 'text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]'
            }`}
          >
            <span className="text-base leading-none">{item.icon}</span>
            {!collapsed && <span>{item.label}</span>}
          </button>
        ))}
      </div>

      {/* Settings (pinned at the bottom, separate from primary nav) */}
      <div className="px-2 pb-1">
        <button
          onClick={() => onSelectView('settings')}
          title={collapsed ? 'Ayarlar' : undefined}
          className={`flex w-full items-center gap-3 rounded-lg px-3 py-2 text-sm transition ${
            collapsed ? 'justify-center' : ''
          } ${
            view === 'settings'
              ? 'bg-[var(--color-accent)] text-white'
              : 'text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]'
          }`}
        >
          <span className="text-base leading-none">⚙</span>
          {!collapsed && <span>Ayarlar</span>}
        </button>
      </div>

      {/* Collapse toggle */}
      <button
        onClick={() => setCollapsed((v) => !v)}
        title={collapsed ? 'Genişlet' : 'Daralt'}
        className="m-2 flex items-center justify-center rounded-lg py-2 text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
      >
        {collapsed ? '»' : '«'}
      </button>
    </nav>
  )
}
