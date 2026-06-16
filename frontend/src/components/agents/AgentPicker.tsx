import { useEffect, useRef, useState } from 'react'
import type { Agent } from '../../types'
import { AgentAvatar } from './AgentAvatar'

interface Props {
  agents: Agent[]
  value: string
  onChange: (id: string) => void
  placeholder?: string
}

// AgentPicker is a custom dropdown that, unlike a native <select>, renders each
// agent's circular avatar (custom emoji or derived initials) next to its name —
// both in the trigger and the option list. Closes on outside click.
export function AgentPicker({ agents, value, onChange, placeholder = 'Ajan seç' }: Props) {
  const [open, setOpen] = useState(false)
  const rootRef = useRef<HTMLDivElement>(null)
  const selected = agents.find((a) => a.id === value)

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

  return (
    <div ref={rootRef} className="relative">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="flex min-w-40 items-center gap-2 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none hover:border-[var(--color-accent)]"
      >
        {selected ? (
          <>
            <AgentAvatar agent={selected} size={20} />
            <span className="truncate">{selected.name}</span>
          </>
        ) : (
          <span className="text-[var(--color-text-dim)]">{placeholder}</span>
        )}
        <span className="ml-auto text-[var(--color-text-dim)]">▾</span>
      </button>
      {open && (
        <div className="absolute z-20 mt-1 max-h-60 w-full min-w-44 overflow-y-auto rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] py-1 shadow-lg">
          {agents.length === 0 && (
            <div className="px-3 py-2 text-sm text-[var(--color-text-dim)]">Ajan yok</div>
          )}
          {agents.map((a) => (
            <button
              key={a.id}
              type="button"
              onClick={() => {
                onChange(a.id)
                setOpen(false)
              }}
              className={`flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm hover:bg-[var(--color-bg)] ${
                a.id === value ? 'text-[var(--color-accent)]' : ''
              }`}
            >
              <AgentAvatar agent={a} size={20} />
              <span className="truncate">{a.name}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
