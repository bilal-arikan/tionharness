// A live status strip for any READ-ONLY session with a turn in flight — a
// coordinator-tree worker, a schedule run, a flow run. Without it such a session
// looks finished — the transcript stops growing and there is no composer to hint
// that work is ongoing. It shows a running indicator plus a "Durdur" control that
// stops the in-flight turn (the server resolves the run id from the session, and
// falls back to Runtime.CancelSession for autonomous turns that never registered
// a chat run). Rendered only while a turn streams; when the session is idle the
// transcript itself carries the result.
import { Loader2, Square } from 'lucide-react'
import { ComposerCard } from './ComposerCard'
import { BTN_STOP_COMPACT } from './composer/buttonStyles'

interface Props {
  // True when this session is part of a coordinator tree; only changes the
  // wording ("Worker" vs the generic label), not the control itself.
  coordinatorTree?: boolean
  // Stop the in-flight turn (chat.stopTurn on the open session).
  onStop: () => void
}

export function WorkerStatusStrip({ coordinatorTree, onStop }: Props) {
  const label = coordinatorTree ? 'Worker çalışıyor' : 'Oturum çalışıyor'
  return (
    <ComposerCard tone="plain" className="flex items-center gap-2 px-3 py-2 text-sm">
      <Loader2 size={15} className="shrink-0 animate-spin text-[var(--color-accent)]" />
      <span className="min-w-0 flex-1 truncate text-[var(--color-text)]">
        <span className="font-medium text-[var(--color-accent)]">{label}</span>
        <span className="text-[var(--color-text-dim)]"> — turu sürüyor</span>
      </span>
      <button onClick={onStop} title="Bu oturumun süren turunu durdur" className={BTN_STOP_COMPACT}>
        <Square size={13} />
        Durdur
      </button>
    </ComposerCard>
  )
}
