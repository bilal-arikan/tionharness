import { useState } from 'react'
import type { TurnStep, TodoItem } from '../../types'

interface Props {
  step: TurnStep
}

// Glyph + colour per todo status.
const MARK: Record<TodoItem['status'], { icon: string; cls: string }> = {
  completed: { icon: '✓', cls: 'text-green-400 line-through opacity-70' },
  in_progress: { icon: '◐', cls: 'text-[var(--color-accent)] font-medium' },
  pending: { icon: '○', cls: 'text-[var(--color-text-dim)]' },
}

// Items come from the typed `todos` field (kind 'todo'); fall back to parsing the
// tool input for traces persisted before 'todo' was a first-class step kind.
function readTodos(step: TurnStep): TodoItem[] {
  if (step.todos?.length) return step.todos
  const input = step.input
  if (!input || typeof input !== 'object') return []
  const todos = (input as { todos?: unknown }).todos
  if (!Array.isArray(todos)) return []
  return todos.filter(
    (t): t is TodoItem => !!t && typeof (t as TodoItem).content === 'string',
  )
}

// TodoCard renders a working checklist instead of a generic tool card. Completed
// items are struck through; the active item is accented — mirroring the task-list
// affordance in External Agent / Claude Code.
export function TodoCard({ step }: Props) {
  // Default collapsed: the pinned TodoPanel already surfaces the current list,
  // so the inline trace card stays compact (header only) until expanded.
  const [open, setOpen] = useState(false)
  const todos = readTodos(step)
  if (!todos.length) return null
  const done = todos.filter((t) => t.status === 'completed').length

  return (
    <div className="overflow-hidden rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)]">
      <button
        onClick={() => setOpen((o) => !o)}
        className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-xs text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)]"
      >
        <span>✅</span>
        <span className="font-medium text-[var(--color-text)]">Görev Listesi</span>
        <span className="ml-auto tabular-nums">
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
  )
}
