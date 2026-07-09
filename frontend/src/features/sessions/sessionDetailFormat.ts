import { roleColor } from '@/shared/lib/palette'

// ---- formatting ----

export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  const units = ['KB', 'MB', 'GB']
  let v = bytes / 1024
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(v < 10 ? 1 : 0)} ${units[i]}`
}

export function formatTokens(t: number): string {
  if (t < 1000) return String(t)
  return `${(t / 1000).toFixed(1)}k`
}

// formatElapsed renders a running duration in seconds as "42sn" / "3d 5sn".
export function formatElapsed(sec: number): string {
  if (sec < 60) return `${sec}sn`
  const m = Math.floor(sec / 60)
  const s = sec % 60
  if (m < 60) return `${m}d ${s}sn`
  const h = Math.floor(m / 60)
  return `${h}s ${m % 60}d`
}

// pctOf returns n as a whole-number percent of total (0 when total is 0).
export function pctOf(n: number, total: number): number {
  return total > 0 ? Math.round((n / total) * 100) : 0
}

// CACHE_TTL_SEC is the prompt-cache warm window in seconds. Mirrors the backend
// constants: providers.cacheTTL ("1h" ephemeral breakpoint) and
// agent.promptEpochAdoptAfter (time.Hour). Kept in sync manually — the backend
// hardcodes a single 1h TTL, so no field is sent over the wire.
export const CACHE_TTL_SEC = 3600

// cacheRemaining returns the seconds left before the prompt cache goes cold,
// derived purely from the session's last-activity timestamp. Negative once the
// cache has expired; -1 when there is no activity timestamp yet.
export function cacheRemaining(updatedAtSec: number, nowSec: number): number {
  if (!updatedAtSec) return -1
  return CACHE_TTL_SEC - (nowSec - updatedAtSec)
}

// formatCountdown renders remaining seconds as mm:ss, clamped at 0:00.
export function formatCountdown(sec: number): string {
  const s = Math.max(0, sec)
  const m = Math.floor(s / 60)
  const r = s % 60
  return `${m}:${String(r).padStart(2, '0')}`
}

// fillerColor maps a context bucket role to a stable segment colour for the
// usage bar and legend (hues live in the shared categorical palette).
export const fillerColor = roleColor

export function formatDate(unixSec: number): string {
  if (!unixSec) return '—'
  return new Date(unixSec * 1000).toLocaleString('tr-TR', {
    day: '2-digit',
    month: 'short',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}
