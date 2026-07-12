import { useState } from 'react'
import { Loader2, CheckCircle2, ListChecks, Square, ChevronDown, ChevronRight } from 'lucide-react'
import type { SessionProgress } from '@/types'
import { formatDate } from './sessionDetailFormat'

// ProgressCard renders the session's persistent progress file read-only: a count
// summary, each checklist item with its status marker (and optional category),
// and the most recent rolling-log lines. Surfaces the note-taking the agent
// maintains via todo_write.
//
// The file is PER-SESSION (keyed by session id under the store, see
// internal/progress + Runtime.ProgressDir): each session keeps its own checklist,
// so it never leaks into another session sharing the same working directory.
export function ProgressCard({ progress }: { progress: SessionProgress }) {
  const rec = progress.record!
  const total = rec.todos.length
  const done = rec.todos.filter((t) => t.status === 'completed').length
  const log = (rec.log ?? []).slice(-3).reverse()
  // Collapse state (persisted) — the task list can get long, so let it fold.
  const [open, setOpen] = useState(
    () => localStorage.getItem('tionswarm.sessionTasksOpen') !== '0',
  )
  const toggle = () =>
    setOpen((v) => {
      const next = !v
      localStorage.setItem('tionswarm.sessionTasksOpen', next ? '1' : '0')
      return next
    })
  return (
    <section>
      <button
        onClick={toggle}
        aria-expanded={open}
        className="mb-2 flex w-full items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)] opacity-70 transition hover:text-[var(--color-accent)] hover:opacity-100"
      >
        {open ? <ChevronDown size={12} className="shrink-0" /> : <ChevronRight size={12} className="shrink-0" />}
        <ListChecks size={12} className="shrink-0" />
        <span>Görev Listesi · {done}/{total}</span>
        {rec.updatedAt > 0 && (
          <span className="ml-auto font-normal normal-case opacity-80" title="Son güncelleme">
            {formatDate(rec.updatedAt)}
          </span>
        )}
      </button>
      {open && (
      <>
      <div className="flex flex-col gap-1 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-2.5 py-2">
        {rec.todos.map((t, i) => (
          <div key={i} className="flex items-start gap-1.5 text-[11px] leading-relaxed">
            {t.status === 'completed' ? (
              <CheckCircle2 size={13} className="mt-px shrink-0" style={{ color: 'var(--color-success)' }} />
            ) : t.status === 'in_progress' ? (
              <Loader2 size={13} className="mt-px shrink-0 text-[var(--color-accent)]" />
            ) : (
              <Square size={13} className="mt-px shrink-0 text-[var(--color-text-dim)]" />
            )}
            <span className={t.status === 'completed' ? 'text-[var(--color-text-dim)] line-through' : 'text-[var(--color-text)]'}>
              {t.content}
              {t.category && <span className="ml-1 opacity-50">· {t.category}</span>}
            </span>
          </div>
        ))}
      </div>
      {log.length > 0 && (
        <div className="mt-1.5 flex flex-col gap-0.5">
          {log.map((l, i) => (
            <div key={i} className="truncate text-[10px] text-[var(--color-text-dim)] opacity-70" title={l.note}>
              {l.note}
            </div>
          ))}
        </div>
      )}
      </>
      )}
    </section>
  )
}
