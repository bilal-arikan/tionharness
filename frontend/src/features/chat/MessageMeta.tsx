import { useEffect, useState } from 'react'
import { Flame, Snowflake } from 'lucide-react'
import type { MessageUsage } from '@/types'
import { clockTime, fullDateTime, formatDuration, formatDurationMs } from '@/shared/lib/time'
import { serverNow } from '@/shared/lib/serverClock'

// CacheWarmthDot is the one-glyph prompt-cache verdict for a finished turn, shown
// in the message footer so a transcript can be SCANNED for cold turns instead of
// opening every debug panel. Derived purely from the usage already persisted on
// the message — no request.
//
// Renders nothing without cache evidence: a read means warm, a write with no read
// means the prefix was (re)paid cold, and neither means the provider reported no
// cache counters at all (OpenRouter bills a cold prefix as plain input, and a
// non-caching model has none) — claiming "cold" there would be a guess.
export function CacheWarmthDot({ usage }: { usage?: MessageUsage }) {
  const read = usage?.cacheRead ?? 0
  const write = usage?.cacheWrite ?? 0
  if (read > 0) {
    return (
      <span
        title="Sıcak: bu tur cache'li öneki yeniden kullandı (ucuz)"
        className="text-[var(--color-warning)] opacity-70"
      >
        <Flame size={10} />
      </span>
    )
  }
  if (write > 0) {
    return (
      <span
        title="Soğuk: cache öneki bu turda baştan yazıldı (pahalı). Sebebi için debug panelini aç."
        className="text-[var(--color-text-dim)] opacity-70"
      >
        <Snowflake size={10} />
      </span>
    )
  }
  return null
}

// MessageTime renders a message's send time as a short clock label, with the
// full date+time available on hover. Lives in the turn footer, outside the bubble,
// so it always sits on the neutral chat background.
export function MessageTime({ unixSec }: { unixSec: number }) {
  if (!unixSec) return null
  return (
    <span
      title={fullDateTime(unixSec)}
      className="text-[10px] text-[var(--color-text-dim)] opacity-70"
    >
      {clockTime(unixSec)}
    </span>
  )
}

// TurnDuration shows how long a completed assistant turn took, e.g. "⏱ 2 dk 15 sn".
// `ms` is the SERVER-measured wall clock of the turn (Message.durationMs), so the
// label reports what the backend actually timed rather than a client-side delta
// between two timestamps. `derived` flags the legacy fallback (pre-durationMs
// messages, where the value is reconstructed from the createdAt gap) so the
// tooltip does not overstate its accuracy. Hidden for unknown spans.
export function TurnDuration({ ms, derived = false }: { ms: number; derived?: boolean }) {
  if (!ms || ms <= 0) return null
  return (
    <span
      title={
        derived
          ? 'Agentın bu yanıtı üretme süresi (yaklaşık — eski mesaj, mesaj zamanlarından türetildi)'
          : 'Agentın bu yanıtı üretme süresi (sunucuda ölçüldü)'
      }
      className="text-[10px] text-[var(--color-text-dim)] opacity-70"
    >
      ⏱ {formatDurationMs(ms)}
      {derived && <span className="opacity-60"> ~</span>}
    </span>
  )
}

// LiveTimer ticks once a second, showing the elapsed time since the in-flight
// turn started. Used on the live assistant bubble while it streams. Both ends of
// the subtraction are on the SERVER's clock: `startUnixSec` comes from the hub's
// agent_start frame and "now" from serverNow(), so a skewed client clock cannot
// distort the reading.
export function LiveTimer({ startUnixSec }: { startUnixSec: number }) {
  const [now, setNow] = useState(() => serverNow())
  useEffect(() => {
    const id = setInterval(() => setNow(serverNow()), 1000)
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
