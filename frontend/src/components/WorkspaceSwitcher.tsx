import { useEffect, useRef, useState } from 'react'
import type { Workspace } from '../types'
import { WorkspaceCreateModal, type NewWorkspaceData } from './WorkspaceCreateModal'

interface Props {
  workspaces: Workspace[]
  activeId: string | null
  unreadIds: Set<string>
  onSwitch: (id: string) => void
  onCreate: (data: NewWorkspaceData) => void
}

export function WorkspaceSwitcher({ workspaces, activeId, unreadIds, onSwitch, onCreate }: Props) {
  const [open, setOpen] = useState(false)
  const [showCreate, setShowCreate] = useState(false)
  const rootRef = useRef<HTMLDivElement>(null)

  const active = workspaces.find((w) => w.id === activeId)
  // Any non-active workspace with pending activity → the trigger shows a dot.
  const hasUnread = unreadIds.size > 0

  // Close the dropdown when clicking anywhere outside it.
  useEffect(() => {
    if (!open) return
    const onDown = (e: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', onDown)
    return () => document.removeEventListener('mousedown', onDown)
  }, [open])

  const create = (data: NewWorkspaceData) => {
    onCreate(data)
    setShowCreate(false)
    setOpen(false)
  }

  return (
    <div ref={rootRef} className="relative border-b border-[var(--color-border)] px-3 py-3">
      <button
        onClick={() => setOpen((v) => !v)}
        className="flex w-full items-center justify-between rounded-lg bg-[var(--color-surface-2)] px-3 py-2 text-sm hover:opacity-90"
      >
        <span className="flex items-center gap-2 truncate">
          <span
            className="relative flex h-5 w-5 shrink-0 items-center justify-center rounded text-sm"
            style={active?.color ? { backgroundColor: active.color + '33' } : undefined}
          >
            {active?.icon || '⬡'}
            {hasUnread && (
              <span className="absolute -right-1 -top-1 h-2 w-2 rounded-full bg-[var(--color-accent)] ring-2 ring-[var(--color-surface-2)]" />
            )}
          </span>
          <span className="truncate font-medium">{active?.name || 'Workspace seç'}</span>
        </span>
        <span className="text-xs text-[var(--color-text-dim)]">▾</span>
      </button>

      {open && (
        <div className="absolute left-3 right-3 z-10 mt-1 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-1 shadow-xl">
          {workspaces.map((w) => (
            <button
              key={w.id}
              onClick={() => {
                onSwitch(w.id)
                setOpen(false)
              }}
              className={`flex w-full items-center gap-2 rounded px-3 py-2 text-left text-sm transition ${
                w.id === activeId
                  ? 'bg-[var(--color-accent-soft)]'
                  : 'hover:bg-[var(--color-surface-2)]'
              }`}
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
          ))}

          <div className="my-1 border-t border-[var(--color-border)]" />

          <button
            onClick={() => setShowCreate(true)}
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
