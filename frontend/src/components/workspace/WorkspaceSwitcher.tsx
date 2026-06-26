import { useState, type ReactNode } from 'react'
import { ExternalLink, Trash2, Star } from 'lucide-react'
import type { Workspace } from '../../types'
import { buildRoute } from '../../lib/url'
import { useOutsideClick } from '../../hooks/useOutsideClick'
import { WorkspaceCreateModal, type NewWorkspaceData } from './WorkspaceCreateModal'

interface Props {
  workspaces: Workspace[]
  activeId: string | null
  unreadIds: Set<string>
  // Active workspace rollup: any view busy / any unsaved edit (shown on the label).
  activeBusy?: boolean
  activeDirty?: boolean
  // Startup-favorite workspace + toggle (star in each dropdown row).
  favoriteId?: string | null
  onToggleFavorite?: (id: string) => void
  onSwitch: (id: string) => void
  onCreate: (data: NewWorkspaceData) => void
  onDelete: (id: string) => void
  // Optional control rendered inline to the right of the trigger (e.g. the rail
  // collapse toggle), so it shares the workspace row instead of a separate line.
  trailing?: ReactNode
}

// Open a workspace in a fresh window scoped to it (#/w/{id}/chat), without
// disturbing this window's selection. In the native desktop app (WebView2),
// window.open would leak to the system browser, so we call the host bridge
// (swarmgoOpenWindow) to spawn a real SwarmGo window instead; in a browser we
// keep the standard new-window behaviour.
function openInNewWindow(id: string) {
  const route = buildRoute({ workspaceId: id, view: 'chat', id: null })
  const w = window as unknown as {
    chrome?: { webview?: unknown }
    swarmgoOpenWindow?: (route: string) => void
  }
  if (w.chrome?.webview && typeof w.swarmgoOpenWindow === 'function') {
    w.swarmgoOpenWindow(route)
    return
  }
  window.open(`${window.location.origin}${window.location.pathname}#${route}`, '_blank', 'noopener')
}

export function WorkspaceSwitcher({ workspaces, activeId, unreadIds, activeBusy, activeDirty, favoriteId, onToggleFavorite, onSwitch, onCreate, onDelete, trailing }: Props) {
  const [open, setOpen] = useState(false)
  const [showCreate, setShowCreate] = useState(false)
  // Close the dropdown when clicking anywhere outside it (detached while closed).
  const rootRef = useOutsideClick<HTMLDivElement>(() => setOpen(false), open)

  const active = workspaces.find((w) => w.id === activeId)
  // Any non-active workspace with pending activity → the trigger shows a dot.
  const hasUnread = unreadIds.size > 0

  const create = (data: NewWorkspaceData) => {
    onCreate(data)
    setShowCreate(false)
    setOpen(false)
  }

  return (
    <div ref={rootRef} className="relative border-b border-[var(--color-border)] px-3 py-3">
      <div className="flex items-center gap-1">
      <button
        onClick={() => setOpen((v) => !v)}
        data-testid="workspace-switcher"
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-label={active?.name ? `Workspace: ${active.name}` : 'Workspace seç'}
        className="flex min-w-0 flex-1 items-center justify-between rounded-lg bg-[var(--color-surface-2)] px-3 py-2 text-sm hover:opacity-90"
      >
        <span className="flex min-w-0 items-center gap-2">
          <span
            className="relative flex h-5 w-5 shrink-0 items-center justify-center rounded text-sm"
            style={active?.color ? { backgroundColor: active.color + '33' } : undefined}
          >
            {active?.icon || '⬡'}
            {hasUnread && (
              <span className="absolute -right-1 -top-1 h-2 w-2 rounded-full bg-[var(--color-accent)] ring-2 ring-[var(--color-surface-2)]" />
            )}
          </span>
          {/* Active-workspace signals sit inline next to the icon so the parent's
              `truncate` (overflow:hidden) never clips a corner-positioned dot. */}
          {(activeBusy || activeDirty) && (
            <span className="flex shrink-0 items-center gap-1">
              {activeDirty && (
                <span className="h-2 w-2 rounded-full bg-[var(--color-warning)]" title="Kaydedilmemiş değişiklik" />
              )}
              {activeBusy && (
                <span
                  className="h-2 w-2 animate-pulse rounded-full bg-[var(--color-accent)]"
                  title="İşlem sürüyor"
                />
              )}
            </span>
          )}
          <span className="truncate font-medium">{active?.name || 'Workspace seç'}</span>
        </span>
        <span className="text-xs text-[var(--color-text-dim)]">▾</span>
      </button>
        {trailing}
      </div>

      {open && (
        <div
          role="listbox"
          aria-label="Workspace listesi"
          data-testid="workspace-menu"
          className="absolute left-3 z-20 mt-1 w-72 max-w-[calc(100vw-2rem)] rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-1 shadow-xl"
        >
          {workspaces.map((w) => (
            <div
              key={w.id}
              data-testid="workspace-row"
              data-workspace-id={w.id}
              className={`group flex w-full items-center gap-1 rounded pr-1 text-sm transition ${
                w.id === activeId
                  ? 'bg-[var(--color-accent-soft)]'
                  : 'hover:bg-[var(--color-surface-2)]'
              }`}
            >
              <button
                onClick={() => {
                  onSwitch(w.id)
                  setOpen(false)
                }}
                role="option"
                aria-selected={w.id === activeId}
                data-testid="workspace-switch"
                aria-label={`Workspace’e geç: ${w.name || 'İsimsiz'}`}
                className="flex flex-1 items-center gap-2 rounded px-3 py-2 text-left"
              >
                <span
                  className="flex h-5 w-5 shrink-0 items-center justify-center rounded text-sm"
                  style={w.color ? { backgroundColor: w.color + '33' } : undefined}
                >
                  {w.icon || '⬡'}
                </span>
                <span className="flex-1 truncate">{w.name || 'İsimsiz'}</span>
                {unreadIds.has(w.id) && (
                  <span className="h-2 w-2 shrink-0 rounded-full bg-[var(--color-accent)]" title="Yeni etkinlik" />
                )}
              </button>
              {onToggleFavorite && (
                <button
                  onClick={() => onToggleFavorite(w.id)}
                  title={favoriteId === w.id ? 'Başlangıç workspace’i (kaldır)' : 'Başlangıçta bunu aç'}
                  className={`flex h-7 w-7 shrink-0 items-center justify-center rounded transition hover:bg-[var(--color-surface)] ${
                    favoriteId === w.id
                      ? 'text-[var(--color-accent)]'
                      : 'text-[var(--color-text-dim)] opacity-0 hover:text-[var(--color-accent)] focus:opacity-100 group-hover:opacity-100'
                  }`}
                >
                  <Star size={14} strokeWidth={2} fill={favoriteId === w.id ? 'currentColor' : 'none'} />
                </button>
              )}
              <button
                onClick={() => openInNewWindow(w.id)}
                title="Ayrı pencerede aç"
                className="flex h-7 w-7 shrink-0 items-center justify-center rounded text-[var(--color-text-dim)] opacity-0 transition hover:bg-[var(--color-surface)] hover:text-[var(--color-text)] focus:opacity-100 group-hover:opacity-100"
              >
                <ExternalLink size={14} strokeWidth={2} />
              </button>
              <button
                onClick={() => onDelete(w.id)}
                disabled={workspaces.length <= 1}
                title={workspaces.length <= 1 ? 'Son workspace silinemez' : 'Workspace’i sil'}
                className="flex h-7 w-7 shrink-0 items-center justify-center rounded text-[var(--color-text-dim)] opacity-0 transition hover:bg-[var(--color-surface)] hover:text-[var(--color-danger)] focus:opacity-100 group-hover:opacity-100 disabled:cursor-not-allowed disabled:opacity-30"
              >
                <Trash2 size={14} strokeWidth={2} />
              </button>
            </div>
          ))}

          <div className="my-1 border-t border-[var(--color-border)]" />

          <button
            onClick={() => setShowCreate(true)}
            data-testid="workspace-create"
            aria-label="Yeni workspace"
            className="flex w-full items-center gap-2 rounded px-3 py-2 text-left text-sm text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)]"
          >
            <span>+</span> Yeni workspace
          </button>
        </div>
      )}

      {showCreate && (
        <WorkspaceCreateModal onCreate={create} onClose={() => setShowCreate(false)} />
      )}
    </div>
  )
}
