import { useEffect, useState } from 'react'
import { clockTime, fullDateTime, formatDuration } from '@/shared/lib/time'

// MessageTime renders a message's send time as a short clock label, with the
// full date+time available on hover. Lives in the turn footer, outside the bubble,
// so it always sits on the neutral chat background.
export function MessageTime({ unixSec }: { unixSec: number }) {
  if (!unixSec) return null
  return (
    <span title={fullDateTime(unixSec)} className="text-[10px] text-[var(--color-text-dim)] opacity-70">
      {clockTime(unixSec)}
    </span>
  )
}

// TurnDuration shows how long a completed assistant turn took (end - start),
// e.g. "⏱ 2 dk 15 sn". Hidden for sub-second / unknown spans.
export function TurnDuration({ seconds }: { seconds: number }) {
  if (!seconds || seconds < 1) return null
  return (
    <span
      title="Agentın bu yanıtı üretme süresi"
      className="text-[10px] text-[var(--color-text-dim)] opacity-70"
    >
      ⏱ {formatDuration(seconds)}
    </span>
  )
}

// LiveTimer ticks once a second, showing the elapsed time since the in-flight
// turn started. Used on the live assistant bubble while it streams.
export function LiveTimer({ startUnixSec }: { startUnixSec: number }) {
  const [now, setNow] = useState(() => Math.floor(Date.now() / 1000))
  useEffect(() => {
    const id = setInterval(() => setNow(Math.floor(Date.now() / 1000)), 1000)
    return () => clearInterval(id)
  }, [])
  if (!startUnixSec) return null
  return (
    <span
      title="Agent ne zamandır çalışıyor"
      className="text-[10px] text-[var(--color-accent)] opacity-80"
    >
      ⏱ {formatDuration(now - startUnixSec)}
    </span>
  )
}
