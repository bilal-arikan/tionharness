// A waiting banner shown above the composer while a session is paused on a
// pending self-wake (schedule_wake): the agent ended its turn and armed an
// automatic re-invocation after a delay. Without this the session would look
// finished. It surfaces the agent's reason, a live countdown to the wake, and a
// "Durdur" control that disarms the wake (POST /api/chat/wake/cancel).
import { useEffect, useState } from 'react'
import { AlarmClock, X } from 'lucide-react'

interface Props {
  // The agent's stated reason for waiting (may be empty).
  reason: string
  // Unix seconds when the wake fires (0 if unknown — countdown is then hidden).
  fireAt: number
  // Durdur: disarm the pending wake.
  onCancel: () => void
}

// remainingLabel renders the seconds left until fireAt as a compact "~Xs" / "~Xm Ys".
function remainingLabel(fireAt: number, nowSec: number): string | null {
  if (fireAt <= 0) return null
  const left = fireAt - nowSec
  if (left <= 0) return 'birazdan'
  if (left < 60) return `~${left}sn`
  const m = Math.floor(left / 60)
  const s = left % 60
  return s ? `~${m}dk ${s}sn` : `~${m}dk`
}

// WakeWaitBanner shows the "waiting to auto-resume" state for a session with a
// pending self-wake, plus a Durdur control.
export function WakeWaitBanner({ reason, fireAt, onCancel }: Props) {
  // Tick once a second so the countdown stays live without a parent re-render.
  const [now, setNow] = useState(() => Math.floor(Date.now() / 1000))
  useEffect(() => {
    const t = setInterval(() => setNow(Math.floor(Date.now() / 1000)), 1000)
    return () => clearInterval(t)
  }, [])

  const left = remainingLabel(fireAt, now)
  return (
    <div className="border-t border-[var(--color-border)] bg-[var(--color-surface)] px-6 pt-3">
      <div className="flex items-center gap-2 rounded-lg border border-[color-mix(in_srgb,var(--color-accent)_35%,var(--color-border))] bg-[var(--color-accent-soft)] px-3 py-2 text-sm">
        <AlarmClock size={15} className="shrink-0 animate-pulse text-[var(--color-accent)]" />
        <span className="min-w-0 flex-1 truncate text-[var(--color-text)]">
          <span className="font-medium text-[var(--color-accent)]">Otomatik devam bekleniyor</span>
          {left && <span className="text-[var(--color-text-dim)]"> · {left}</span>}
          {reason && <span className="text-[var(--color-text-dim)]"> — {reason}</span>}
        </span>
        <button
          onClick={onCancel}
          title="Otomatik uyandırmayı durdur"
          className="inline-flex shrink-0 items-center gap-1 rounded-md border border-[var(--color-border)] bg-[var(--color-surface-2)] px-2 py-1 text-xs font-medium text-[var(--color-text)] transition hover:border-[var(--color-danger)] hover:text-[var(--color-danger)]"
        >
          <X size={13} />
          Durdur
        </button>
      </div>
    </div>
  )
}
