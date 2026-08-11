// A live status strip for a READ-ONLY coordinator-tree session (worker or
// sub-coordinator). Without it a worker whose turn is still streaming looks
// finished — the transcript stops growing and there is no composer to hint that
// work is ongoing. It shows a running indicator plus a "Durdur" control that
// stops the worker's in-flight turn (the server resolves the run id from the
// session, so no run id is needed here). Rendered only while a turn streams; when
// the worker is idle the transcript itself carries the result.
import { Loader2, Square } from 'lucide-react'
import { ComposerCard } from './ComposerCard'

interface Props {
  // Stop the worker's in-flight turn (chat.stopTurn on the open session).
  onStop: () => void
}

export function WorkerStatusStrip({ onStop }: Props) {
  return (
    <ComposerCard tone="plain" className="flex items-center gap-2 px-3 py-2 text-sm">
      <Loader2 size={15} className="shrink-0 animate-spin text-[var(--color-accent)]" />
      <span className="min-w-0 flex-1 truncate text-[var(--color-text)]">
        <span className="font-medium text-[var(--color-accent)]">Worker çalışıyor</span>
        <span className="text-[var(--color-text-dim)]"> — turu sürüyor</span>
      </span>
      <button
        onClick={onStop}
        title="Bu worker'ın süren turunu durdur"
        className="inline-flex shrink-0 items-center gap-1 rounded-md border border-[var(--color-border)] bg-[var(--color-surface-2)] px-2 py-1 text-xs font-medium text-[var(--color-text)] transition hover:border-[var(--color-danger)] hover:text-[var(--color-danger)]"
      >
        <Square size={13} />
        Durdur
      </button>
    </ComposerCard>
  )
}
