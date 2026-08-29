import { useState } from 'react'
import { X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { TodoItem } from '@/types'
import { ScrollableCard } from '@/shared/components'
import { ComposerCard } from './ComposerCard'

interface Props {
  todos: TodoItem[]
  dismissed: boolean
  onDismiss: () => void
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
// page reloads. It stays pinned while work remains; once complete, the user can
// dismiss that session/list pair with the close button.
export function TodoPanel({ todos, dismissed, onDismiss }: Props) {
  const { t } = useTranslation()
  const done = todos.filter((t) => t.status === 'completed').length
  const allDone = todos.length > 0 && done === todos.length
  const [open, setOpen] = useState(false)

  if (!todos.length || dismissed) return null
  const pct = Math.round((done / todos.length) * 100)

  return (
    <ComposerCard tone="plain" className="overflow-hidden">
      <div className="flex w-full items-center gap-2 text-xs">
        <button
          type="button"
          aria-expanded={open}
          onClick={() => setOpen((o) => !o)}
          className="flex min-w-0 flex-1 items-center gap-2 px-3 py-1.5 text-left hover:bg-[var(--color-surface-2)]"
        >
          <span>{allDone ? '✅' : '📋'}</span>
          <span className="font-medium text-[var(--color-text)]">{t('chat.todos.title')}</span>
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
        {allDone && (
          <button
            type="button"
            aria-label={t('chat.todos.dismiss')}
            title={t('chat.todos.dismiss')}
            onClick={(event) => {
              event.stopPropagation()
              onDismiss()
            }}
            className="mr-1.5 shrink-0 rounded p-1 text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
          >
            <X size={14} aria-hidden="true" />
          </button>
        )}
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
