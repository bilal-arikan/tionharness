import { useState } from 'react'
import type { TodoItem } from '@/types'
import { ScrollableCard } from '@/shared/components'
import { ComposerCard } from './ComposerCard'

interface Props {
  todos: TodoItem[]
}

// Glyph + colour per todo status (mirrors TodoCard).
const MARK: Record<TodoItem['status'], { icon: string; cls: string }> = {
  completed: { icon: '✓', cls: 'text-[var(--color-success)] line-through opacity-70' },
  in_progress: { icon: '◐', cls: 'text-[var(--color-accent)] font-medium' },
  pending: { icon: '○', cls: 'text-[var(--color-text-dim)]' },
}

// TodoPanel pins the session's current checklist just above the composer, like
// the pending-message tray. Unlike the inline TodoCard (which is frozen into one
// turn's trace), this always shows the LATEST todo_write for the session and
// updates as the agent ticks items off, so the list persists across turns and
// page reloads. Minimizable but NOT dismissable: it defaults collapsed (header
// only) and stays pinned; the user can expand/collapse but can never close it.
export function TodoPanel({ todos }: Props) {
  const done = todos.filter((t) => t.status === 'completed').length
  const allDone = todos.length > 0 && done === todos.length
  // Defaults minimized (collapsed): the header still surfaces progress; the user
  // toggles it open on demand. Not dismissable, it stays pinned above the composer.
  const [open, setOpen] = useState(false)

  if (!todos.length) return null
  const pct = Math.round((done / todos.length) * 100)

  return (
    <ComposerCard tone="plain" className="overflow-hidden">
      <div className="flex w-full items-center gap-2 text-xs">
        <button
          onClick={() => setOpen((o) => !o)}
          className="flex min-w-0 flex-1 items-center gap-2 px-3 py-1.5 text-left hover:bg-[var(--color-surface-2)]"
        >
          <span>{allDone ? '✅' : '📋'}</span>
          <span className="font-medium text-[var(--color-text)]">Görev Listesi</span>
          {/* Slim progress bar. */}
          <span className="ml-1 hidden h-1.5 w-24 overflow-hidden rounded-full bg-[var(--color-border)] sm:block">
            <span
              className="block h-full bg-[var(--color-accent)] transition-all"
              style={{ width: `${pct}%` }}
            />
          </span>
          <span className="ml-auto tabular-nums text-[var(--color-text-dim)]">
            {done}/{todos.length}
          </span>
          <span className="shrink-0 opacity-50">{open ? '▾' : '▸'}</span>
        </button>
      </div>

      {open && (
        <ScrollableCard maxH="max-h-[45vh]">
          <ul className="flex flex-col gap-0.5 border-t border-[var(--color-border)] px-3 py-2 text-xs">
            {todos.map((t, i) => {
              const m = MARK[t.status] ?? MARK.pending
              return (
                <li key={i} className={`flex items-start gap-2 ${m.cls}`}>
                  <span className="shrink-0 leading-5">{m.icon}</span>
                  <span className="min-w-0 flex-1 leading-5">{t.content}</span>
                </li>
              )
            })}
          </ul>
        </ScrollableCard>
      )}
    </ComposerCard>
  )
}
