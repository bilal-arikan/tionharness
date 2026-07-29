// A waiting banner shown above the composer while a coordinator session has
// workers still running. Without it the chat looks idle between the coordinator's
// turn ending and the first <task-notification> landing — the conversation IS
// waiting, and this makes that explicit. Each running worker is clickable and
// opens its own session (workers are first-class sessions). See _Docs/47.
import { useEffect, useState } from 'react'
import { Users, Play } from 'lucide-react'
import type { WorkerInfo } from '@/types'
import { serverNow } from '@/shared/lib/serverClock'

// elapsedLabel renders seconds since startedAt as a compact "Xsn" / "Xdk Ysn".
// Returns null when the start time is unknown (startedAt 0) or in the future, so
// the chip simply omits the duration instead of showing "0sn" forever.
function elapsedLabel(startedAt: number, nowSec: number): string | null {
  if (startedAt <= 0) return null
  const secs = nowSec - startedAt
  if (secs < 0) return null
  if (secs < 60) return `${secs}sn`
  const m = Math.floor(secs / 60)
  const s = secs % 60
  return s ? `${m}dk ${s}sn` : `${m}dk`
}

interface Props {
  // Only the running workers; the banner is not rendered when this is empty.
  workers: WorkerInfo[]
  // How many workers under this coordinator have already finished (shown as a
  // "N/total done" hint so long fan-outs show progress).
  doneCount: number
  // Opens a worker's transcript.
  onSelectSession?: (id: string) => void
}

export function WorkerWaitBanner({ workers, doneCount, onSelectSession }: Props) {
  // Tick once a second so each chip's elapsed time stays live without the parent
  // re-rendering (and without any network traffic — the roster itself is
  // event-driven).
  const [now, setNow] = useState(() => serverNow())
  useEffect(() => {
    if (workers.length === 0) return
    const t = setInterval(() => setNow(serverNow()), 1000)
    return () => clearInterval(t)
  }, [workers.length])

  if (workers.length === 0) return null
  const total = workers.length + doneCount

  return (
    <div className="border-t border-[var(--color-border)] bg-[var(--color-surface)] px-6 pt-3">
      <div className="rounded-lg border border-[color-mix(in_srgb,var(--color-accent)_35%,var(--color-border))] bg-[var(--color-accent-soft)] px-3 py-2 text-sm">
        <div className="flex items-center gap-2">
          <Users size={15} className="shrink-0 animate-pulse text-[var(--color-accent)]" />
          <span className="min-w-0 flex-1 truncate text-[var(--color-text)]">
            <span className="font-medium text-[var(--color-accent)]">
              {workers.length} worker çalışıyor
            </span>
            <span className="text-[var(--color-text-dim)]"> — sonuçları bekleniyor</span>
            {doneCount > 0 && (
              <span className="text-[var(--color-text-dim)]"> · {doneCount}/{total} bitti</span>
            )}
          </span>
        </div>
        <ul className="mt-1.5 flex flex-wrap gap-1.5">
          {workers.map((w) => {
            const label = w.agentName || w.title || w.sessionId
            const elapsed = elapsedLabel(w.startedAt, now)
            const inner = (
              <>
                <Play size={10} className="shrink-0 text-[var(--color-accent)]" />
                <span className="max-w-[220px] truncate">{label}</span>
                {elapsed && (
                  <span className="shrink-0 tabular-nums text-[var(--color-text-dim)]">{elapsed}</span>
                )}
              </>
            )
            return (
              <li key={w.sessionId}>
                {onSelectSession ? (
                  <button
                    type="button"
                    onClick={() => onSelectSession(w.sessionId)}
                    title={`${label} — worker oturumunu aç`}
                    className="inline-flex items-center gap-1 rounded-md border border-[var(--color-border)] bg-[var(--color-surface-2)] px-2 py-0.5 text-xs text-[var(--color-text)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
                  >
                    {inner}
                  </button>
                ) : (
                  <span className="inline-flex items-center gap-1 rounded-md border border-[var(--color-border)] bg-[var(--color-surface-2)] px-2 py-0.5 text-xs text-[var(--color-text)]">
                    {inner}
                  </span>
                )}
              </li>
            )
          })}
        </ul>
      </div>
    </div>
  )
}
