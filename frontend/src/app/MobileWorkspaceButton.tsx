import { useState } from 'react'
import type { Workspace } from '@/types'
import { useOutsideClick } from '@/shared/hooks/useOutsideClick'
import {
  WorkspaceCreateModal,
  type NewWorkspaceData,
} from '@/features/workspace/WorkspaceCreateModal'

interface Props {
  workspaces: Workspace[]
  activeId: string | null
  unreadIds?: Set<string>
  // Workspaces (active OR not) with a live run — pulses a green "çalışıyor" dot.
  busyIds?: Set<string>
  onSwitch: (id: string) => void
  onCreate: (data: NewWorkspaceData) => void
}

// MobileWorkspaceButton is the leftmost, pinned item of the bottom MobileNavBar:
// the mobile counterpart of the desktop rail's WorkspaceSwitcher. Tapping it opens
// an UPWARD menu (the bar sits at the screen bottom) to switch between workspaces
// or create a new one. It is intentionally rendered OUTSIDE the nav's horizontal
// scroll strip so its upward popup is not clipped by the scroller's overflow.
export function MobileWorkspaceButton({
  workspaces,
  activeId,
  unreadIds,
  busyIds,
  onSwitch,
  onCreate,
}: Props) {
  const [open, setOpen] = useState(false)
  const [showCreate, setShowCreate] = useState(false)
  const rootRef = useOutsideClick<HTMLDivElement>(() => setOpen(false), open)
  const active = workspaces.find((w) => w.id === activeId)
  const hasUnread = (unreadIds?.size ?? 0) > 0
  // A run is live in some OTHER workspace → the trigger dot pulses.
  const hasOtherBusy = Array.from(busyIds ?? []).some((id) => id !== activeId)

  const create = (data: NewWorkspaceData) => {
    onCreate(data)
    setShowCreate(false)
    setOpen(false)
  }

  return (
    <div
      ref={rootRef}
      className="relative shrink-0 self-stretch border-r border-[var(--color-border)]"
    >
      <button
        onClick={() => setOpen((v) => !v)}
        data-testid="mnav-workspace-switcher"
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-label={active?.name ? `Workspace: ${active.name}` : 'Workspace seç'}
        className="relative flex h-full min-w-[3.75rem] flex-col items-center justify-center gap-0.5 px-2 py-1.5 text-[10px] leading-none text-[var(--color-text-dim)]"
      >
        <span
          className="flex h-[18px] w-[18px] shrink-0 items-center justify-center rounded text-sm leading-none"
          style={
            active?.color
              ? { backgroundColor: `color-mix(in srgb, ${active.color} 20%, transparent)` }
              : undefined
          }
        >
          {active?.icon || '⬡'}
        </span>
        <span className="max-w-[4.5rem] truncate">{active?.name || 'Workspace'}</span>
        {(hasUnread || hasOtherBusy) && (
          <span
            className={`absolute right-2 top-1 h-1.5 w-1.5 rounded-full bg-[var(--color-accent)] ${
              hasOtherBusy ? 'animate-pulse' : ''
            }`}
            title={hasOtherBusy ? 'Başka workspace’te işlem sürüyor' : 'Yeni etkinlik'}
          />
        )}
      </button>

      {open && (
        <div
          role="listbox"
          aria-label="Workspace listesi"
          data-testid="mnav-workspace-menu"
          className="absolute bottom-full left-0 z-50 mb-2 max-h-[60vh] w-60 max-w-[calc(100vw-1rem)] overflow-y-auto rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-1 shadow-xl"
        >
          {workspaces.map((w) => (
            <button
              key={w.id}
              onClick={() => {
                onSwitch(w.id)
                setOpen(false)
              }}
              role="option"
              aria-selected={w.id === activeId}
              data-testid="mnav-workspace-switch"
              className={`flex w-full items-center gap-2 rounded px-3 py-2 text-left text-sm transition ${
                w.id === activeId
                  ? 'bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
                  : 'hover:bg-[var(--color-surface-2)]'
              }`}
            >
              <span
                className="flex h-5 w-5 shrink-0 items-center justify-center rounded text-sm"
                style={
                  w.color
                    ? { backgroundColor: `color-mix(in srgb, ${w.color} 20%, transparent)` }
                    : undefined
                }
              >
                {w.icon || '⬡'}
              </span>
              <span className="flex-1 truncate">{w.name || 'İsimsiz'}</span>
              {/* Explicit run state (green pulse "çalışıyor" wins over the settled
                  "tamamlandı" unread state), mirroring the desktop switcher. */}
              {busyIds?.has(w.id) ? (
                <span
                  className="flex shrink-0 items-center gap-1 text-[10px] font-medium text-[var(--color-success)]"
                  title="İşlem sürüyor"
                >
                  <span className="relative flex h-2 w-2">
                    <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-[var(--color-success)] opacity-75" />
                    <span className="relative inline-flex h-2 w-2 rounded-full bg-[var(--color-success)]" />
                  </span>
                  çalışıyor
                </span>
              ) : (
                unreadIds?.has(w.id) && (
                  <span
                    className="flex shrink-0 items-center gap-1 text-[10px] font-medium text-[var(--color-accent)]"
                    title="Tamamlandı — görülmemiş etkinlik"
                  >
                    <span className="h-2 w-2 rounded-full bg-[var(--color-accent)]" />
                    tamamlandı
                  </span>
                )
              )}
            </button>
          ))}

          <div className="my-1 border-t border-[var(--color-border)]" />

          <button
            onClick={() => setShowCreate(true)}
            data-testid="mnav-workspace-create"
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
