import { useState } from 'react'
import type { Workspace } from '../types'

interface Props {
  workspaces: Workspace[]
  activeId: string | null
  onSwitch: (id: string) => void
  onCreate: (name: string) => void
}

export function WorkspaceSwitcher({ workspaces, activeId, onSwitch, onCreate }: Props) {
  const [open, setOpen] = useState(false)
  const [creating, setCreating] = useState(false)
  const [name, setName] = useState('')

  const active = workspaces.find((w) => w.id === activeId)

  const submit = () => {
    if (!name.trim()) return
    onCreate(name.trim())
    setName('')
    setCreating(false)
    setOpen(false)
  }

  return (
    <div className="relative border-b border-[var(--color-border)] px-3 py-3">
      <button
        onClick={() => setOpen((v) => !v)}
        className="flex w-full items-center justify-between rounded-lg bg-[var(--color-surface-2)] px-3 py-2 text-sm hover:opacity-90"
      >
        <span className="flex items-center gap-2 truncate">
          <span
            className="flex h-5 w-5 shrink-0 items-center justify-center rounded text-sm"
            style={active?.color ? { backgroundColor: active.color + '33' } : undefined}
          >
            {active?.icon || '⬡'}
          </span>
          <span className="truncate font-medium">{active?.name ?? 'Workspace seç'}</span>
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
              <span className="truncate">{w.name}</span>
            </button>
          ))}

          <div className="my-1 border-t border-[var(--color-border)]" />

          {creating ? (
            <div className="flex gap-1 p-1">
              <input
                autoFocus
                value={name}
                onChange={(e) => setName(e.target.value)}
                onKeyDown={(e) => e.key === 'Enter' && submit()}
                placeholder="Workspace adı"
                className="flex-1 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none focus:border-[var(--color-accent)]"
              />
              <button
                onClick={submit}
                className="rounded bg-[var(--color-accent)] px-2 text-sm text-white"
              >
                ✓
              </button>
            </div>
          ) : (
            <button
              onClick={() => setCreating(true)}
              className="flex w-full items-center gap-2 rounded px-3 py-2 text-left text-sm text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)]"
            >
              <span>+</span> Yeni workspace
            </button>
          )}
        </div>
      )}
    </div>
  )
}
