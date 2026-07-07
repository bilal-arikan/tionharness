import { Loader2, CheckCircle2, ListChecks, Square } from 'lucide-react'
import type { SessionProgress } from '@/types'

// ProgressCard renders the session's persistent progress file read-only: a count
// summary, each checklist item with its status marker (and optional category),
// and the most recent rolling-log lines. Surfaces the cross-session note-taking
// that the agent maintains via todo_write.
export function ProgressCard({ progress }: { progress: SessionProgress }) {
  const rec = progress.record!
  const total = rec.todos.length
  const done = rec.todos.filter((t) => t.status === 'completed').length
  const log = (rec.log ?? []).slice(-3).reverse()
  return (
    <section>
      <div className="mb-2 flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)] opacity-70">
        <ListChecks size={12} className="shrink-0" />
        <span>Kalıcı ilerleme · {done}/{total}</span>
      </div>
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
    </section>
  )
}
