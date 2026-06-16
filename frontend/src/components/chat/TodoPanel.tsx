import { useState } from 'react'
import type { TodoItem } from '../../types'

interface Props {
  todos: TodoItem[]
}

// Glyph + colour per todo status (mirrors TodoCard).
const MARK: Record<TodoItem['status'], { icon: string; cls: string }> = {
  completed: { icon: '✓', cls: 'text-green-400 line-through opacity-70' },
  in_progress: { icon: '◐', cls: 'text-[var(--color-accent)] font-medium' },
  pending: { icon: '○', cls: 'text-[var(--color-text-dim)]' },
}

// TodoPanel pins the session's current checklist just above the composer, like
// the pending-message tray. Unlike the inline TodoCard (which is frozen into one
// turn's trace), this always shows the LATEST todo_write for the session and
// updates as the agent ticks items off — so the list persists across turns and
// page reloads. Collapsible; defaults open while work is in progress.
export function TodoPanel({ todos }: Props) {
  const done = todos.filter((t) => t.status === 'completed').length
  const allDone = todos.length > 0 && done === todos.length
  const [open, setOpen] = useState(true)
  if (!todos.length) return null
  const pct = Math.round((done / todos.length) * 100)

  return (
    <div className="border-t border-[var(--color-border)] bg-[var(--color-surface)] px-6 pt-3">
      <div className="overflow-hidden rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)]">
        <button
          onClick={() => setOpen((o) => !o)}
          className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-xs hover:bg-[var(--color-surface)]"
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

        {open && (
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
        )}
      </div>
    </div>
  )
}
