import { useEffect, useMemo, useRef, useState } from 'react'
import type { LucideIcon } from 'lucide-react'
import { Search } from 'lucide-react'
import { ModalOverlay } from './ModalOverlay'

// A single palette entry. `run` is fired on select; the palette closes first so
// navigation/side-effects happen against a clean UI.
export interface Command {
  id: string
  label: string
  hint?: string
  group?: string
  icon?: LucideIcon
  // Extra text folded into the fuzzy match (aliases, the raw view key, etc.).
  keywords?: string
  run: () => void
}

interface Props {
  open: boolean
  onClose: () => void
  commands: Command[]
  placeholder?: string
}

// CommandPalette — a generic ⌘K/Ctrl+K launcher. The owner supplies the command
// list (navigate to a view, switch workspace, …) and the open/close state; the
// palette handles filtering, keyboard navigation and rendering. Substring match
// (not a heavy fuzzy lib) keeps it dependency-free and predictable.
export function CommandPalette({ open, onClose, commands, placeholder = 'Komut ara…' }: Props) {
  const [q, setQ] = useState('')
  const [active, setActive] = useState(0)
  const inputRef = useRef<HTMLInputElement>(null)
  const listRef = useRef<HTMLDivElement>(null)

  // Reset + focus the input each time the palette opens.
  useEffect(() => {
    if (!open) return
    setQ('')
    setActive(0)
    const id = requestAnimationFrame(() => inputRef.current?.focus())
    return () => cancelAnimationFrame(id)
  }, [open])

  const filtered = useMemo(() => {
    const s = q.trim().toLowerCase()
    if (!s) return commands
    return commands.filter((c) =>
      `${c.label} ${c.hint ?? ''} ${c.keywords ?? ''} ${c.group ?? ''}`.toLowerCase().includes(s),
    )
  }, [q, commands])

  // Keep the highlighted row valid + visible as the filtered set shrinks.
  useEffect(() => {
    setActive((a) => Math.min(a, Math.max(0, filtered.length - 1)))
  }, [filtered.length])
  useEffect(() => {
    listRef.current
      ?.querySelector<HTMLElement>('[data-active="true"]')
      ?.scrollIntoView({ block: 'nearest' })
  }, [active])

  if (!open) return null

  const run = (c?: Command) => {
    if (!c) return
    onClose()
    c.run()
  }

  const onKey = (e: React.KeyboardEvent) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setActive((a) => Math.min(a + 1, filtered.length - 1))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setActive((a) => Math.max(a - 1, 0))
    } else if (e.key === 'Enter') {
      e.preventDefault()
      run(filtered[active])
    }
  }

  return (
    <ModalOverlay onClose={onClose}>
      <div
        role="dialog"
        aria-label="Komut paleti"
        className="flex max-h-[70vh] w-full max-w-lg flex-col overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] shadow-[var(--shadow-lg)]"
      >
        <div className="flex items-center gap-2 border-b border-[var(--color-border)] px-3">
          <Search size={16} className="shrink-0 text-[var(--color-text-dim)]" />
          <input
            ref={inputRef}
            value={q}
            onChange={(e) => setQ(e.target.value)}
            onKeyDown={onKey}
            placeholder={placeholder}
            className="flex-1 bg-transparent py-3 text-sm outline-none placeholder:text-[var(--color-text-dim)]"
            aria-label={placeholder}
          />
        </div>
        <div ref={listRef} className="min-h-0 flex-1 overflow-y-auto p-1">
          {filtered.length === 0 ? (
            <div className="px-4 py-8 text-center text-sm text-[var(--color-text-dim)]">
              Eşleşme yok
            </div>
          ) : (
            filtered.map((c, i) => {
              const Icon = c.icon
              const isActive = i === active
              return (
                <button
                  key={c.id}
                  data-active={isActive}
                  onClick={() => run(c)}
                  onMouseMove={() => setActive(i)}
                  className={`flex w-full items-center gap-2.5 rounded-md px-3 py-2 text-left text-sm transition ${
                    isActive ? 'bg-[var(--color-accent-soft)]' : ''
                  }`}
                >
                  {Icon && <Icon size={15} className="shrink-0 text-[var(--color-text-dim)]" />}
                  <span className="min-w-0 flex-1 truncate text-[var(--color-text)]">
                    {c.label}
                  </span>
                  {c.hint && (
                    <span className="shrink-0 truncate text-[11px] text-[var(--color-text-dim)]">
                      {c.hint}
                    </span>
                  )}
                  {c.group && (
                    <span className="shrink-0 rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-[10px] text-[var(--color-text-dim)]">
                      {c.group}
                    </span>
                  )}
                </button>
              )
            })
          )}
        </div>
      </div>
    </ModalOverlay>
  )
}
