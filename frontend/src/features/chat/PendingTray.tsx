// A staged intervention waiting above the composer while a turn streams:
// a queued message (sent when the turn ends) or a steer (live guidance sent
// after a short cancellable delay). Either can be removed before it is applied.
export interface PendingItem {
  id: string
  text: string
  kind: 'queue' | 'steer'
  // The session this intervention belongs to. The tray is filtered to the
  // active session, and queue flush / steer dispatch target this session's
  // turn — so staged items for a background turn never apply to another.
  sid: string
}

interface Props {
  items: PendingItem[]
  onRemove: (id: string) => void
  // Promote a waiting message to dispatch next ("öne al"). Optional.
  onSendNext?: (id: string) => void
  // Clear the whole waiting queue. Optional; shown when 2+ queue items wait.
  onClear?: () => void
  // workersActive: the session's own turn is idle but coordinator workers are still
  // running, so a queued message is being HELD until they drain (backend). Only the
  // reason text changes — the queue is otherwise identical to "waiting behind a
  // streaming turn".
  workersActive?: boolean
}

import { ArrowUp, CornerDownRight, Hourglass, X } from 'lucide-react'
import { ComposerCard } from './ComposerCard'

// PendingTray lists the session's WAITING backend queue (+ any steers) above the
// composer. Queue items show their position (#N), can be promoted to run next,
// removed individually, or cleared all at once.
export function PendingTray({
  items,
  onRemove,
  onSendNext,
  onClear,
  workersActive = false,
}: Props) {
  if (items.length === 0) return null
  const queueCount = items.filter((it) => it.kind === 'queue').length
  // Held behind running workers (not a live turn) → say so, so a chip that just sits
  // there reads as "waiting on the workers" instead of looking stuck.
  const showWorkerHint = workersActive && queueCount > 0
  let qIndex = 0
  return (
    <ComposerCard tone="muted" className="flex flex-col gap-1.5 px-3 py-2">
      <div className="flex items-center justify-between">
        <span className="text-[10px] font-medium uppercase tracking-wide text-[var(--color-text-dim)]">
          {showWorkerHint
            ? 'Workerlar çalışıyor — bitince gönderilecek · silebilirsin'
            : 'Bekleyenler — işleme alınmadan silebilirsin'}
        </span>
        {onClear && queueCount > 1 && (
          <button
            onClick={onClear}
            className="text-[10px] font-medium text-[var(--color-text-dim)] transition hover:text-[var(--color-danger)]"
          >
            Kuyruğu temizle ({queueCount})
          </button>
        )}
      </div>
      {items.map((it) => {
        const pos = it.kind === 'queue' ? ++qIndex : 0
        return (
          <div
            key={it.id}
            className="flex items-center gap-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-2.5 py-1.5 text-sm"
          >
            <span
              className={`inline-flex shrink-0 items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-semibold ${
                it.kind === 'steer'
                  ? 'bg-[color-mix(in_srgb,var(--color-warning)_20%,transparent)] text-[var(--color-warning)]'
                  : 'bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
              }`}
              title={
                it.kind === 'steer'
                  ? 'Canlı yönlendirme (birazdan gönderilecek)'
                  : showWorkerHint
                    ? 'Sıradaki mesaj (workerlar bitince gönderilecek)'
                    : 'Sıradaki mesaj (tur bitince gönderilecek)'
              }
            >
              {it.kind === 'steer' ? <CornerDownRight size={11} /> : <Hourglass size={11} />}
              {it.kind === 'steer' ? 'Yönlendir' : `Sırada #${pos}`}
            </span>
            <span className="min-w-0 flex-1 truncate text-[var(--color-text)]">{it.text}</span>
            {it.kind === 'queue' && onSendNext && pos > 1 && (
              <button
                onClick={() => onSendNext(it.id)}
                title="Öne al (sıradaki tur bunu çalıştırsın)"
                className="shrink-0 rounded p-0.5 text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
              >
                <ArrowUp size={14} />
              </button>
            )}
            <button
              onClick={() => onRemove(it.id)}
              title="Sil (işleme alınmadan)"
              className="shrink-0 rounded p-0.5 text-[var(--color-text-dim)] transition hover:text-[var(--color-danger)]"
            >
              <X size={14} />
            </button>
          </div>
        )
      })}
    </ComposerCard>
  )
}
