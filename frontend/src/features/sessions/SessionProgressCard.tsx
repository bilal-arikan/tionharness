import { Loader2, CheckCircle2, ListChecks, Square } from 'lucide-react'
import type { SessionProgress } from '@/types'
import { formatDate } from './sessionDetailFormat'

// ProgressCard renders the session's persistent progress file read-only: a count
// summary, each checklist item with its status marker (and optional category),
// and the most recent rolling-log lines. Surfaces the cross-session note-taking
// that the agent maintains via todo_write.
//
// The file is keyed by WORKING DIRECTORY, not session (see internal/progress):
// several sessions sharing a project dir share one progress.json. So the record
// may have been last written by ANOTHER session — `sessionId` (the panel's
// session) lets us flag that instead of silently attributing it to this one.
export function ProgressCard({ progress, sessionId }: { progress: SessionProgress; sessionId?: string }) {
  const rec = progress.record!
  const total = rec.todos.length
  const done = rec.todos.filter((t) => t.status === 'completed').length
  const log = (rec.log ?? []).slice(-3).reverse()
  // The dir-scoped file was last written by a DIFFERENT session — this checklist
  // belongs to the shared project dir, not (only) this session.
  const foreign = !!rec.sessionId && !!sessionId && rec.sessionId !== sessionId
  return (
    <section>
      <div className="mb-2 flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)] opacity-70">
        <ListChecks size={12} className="shrink-0" />
        <span>Kalıcı ilerleme · {done}/{total}</span>
        {rec.updatedAt > 0 && (
          <span className="ml-auto font-normal normal-case opacity-80" title="Son güncelleme">
            {formatDate(rec.updatedAt)}
          </span>
        )}
      </div>
      {foreign && (
        <div className="mb-1.5 rounded-md border border-[color-mix(in_srgb,var(--color-warning)_30%,transparent)] bg-[color-mix(in_srgb,var(--color-warning)_8%,transparent)] px-2 py-1 text-[10px] leading-relaxed text-[var(--color-text-dim)]">
          Bu ilerleme <strong>çalışma diziniyle paylaşılıyor</strong> (oturuma değil dizine bağlı) — son yazan oturum <strong>{rec.sessionId}</strong>.
        </div>
      )}
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
