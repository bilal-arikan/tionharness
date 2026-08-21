// Time helpers for the session list: relative "updated" labels and coarse
// recency buckets (today / yesterday / last week / last month / older).

const MIN = 60
const HOUR = 60 * MIN
const DAY = 24 * HOUR

// relativeTime formats a unix-seconds timestamp as a short Turkish relative
// label (e.g. "az önce", "5 dk", "3 sa", "Dün", "12 Haz").
export function relativeTime(unixSec: number, nowMs = Date.now()): string {
  if (!unixSec) return ''
  const diff = Math.max(0, Math.floor(nowMs / 1000) - unixSec)
  if (diff < MIN) return 'az önce'
  if (diff < HOUR) return `${Math.floor(diff / MIN)} dk`
  if (diff < DAY) return `${Math.floor(diff / HOUR)} sa`
  if (diff < 2 * DAY) return 'Dün'
  if (diff < 7 * DAY) return `${Math.floor(diff / DAY)} gün`
  // Older: show a day-month date.
  const d = new Date(unixSec * 1000)
  return d.toLocaleDateString('tr-TR', { day: 'numeric', month: 'short' })
}

// clockTime formats a unix-seconds timestamp as a short local clock ("22:43").
export function clockTime(unixSec: number): string {
  if (!unixSec) return ''
  return new Date(unixSec * 1000).toLocaleTimeString('tr-TR', {
    hour: '2-digit',
    minute: '2-digit',
  })
}

// fullDateTime formats a unix-seconds timestamp as a full local date+time, used
// as a hover title alongside the short clock label.
export function fullDateTime(unixSec: number): string {
  if (!unixSec) return ''
  return new Date(unixSec * 1000).toLocaleString('tr-TR', {
    day: 'numeric',
    month: 'long',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}

// formatDuration renders a span of seconds as a compact Turkish label
// ("45 sn", "2 dk 15 sn", "1 sa 5 dk"). Used for how long an agent turn took
// and for the live elapsed counter of an in-flight turn.
export function formatDuration(seconds: number): string {
  const s = Math.max(0, Math.floor(seconds))
  if (s < MIN) return `${s} sn`
  if (s < HOUR) {
    const m = Math.floor(s / MIN)
    const rem = s % MIN
    return rem ? `${m} dk ${rem} sn` : `${m} dk`
  }
  const h = Math.floor(s / HOUR)
  const m = Math.floor((s % HOUR) / MIN)
  return m ? `${h} sa ${m} dk` : `${h} sa`
}

// formatDurationMs renders a server-measured duration (milliseconds) as a
// compact Turkish label. Short turns keep one decimal ("0.8 sn", "3.4 sn")
// because the backend measures in ms and flooring them to whole seconds would
// throw the precision away; from 10 s up it falls back to formatDuration.
export function formatDurationMs(ms: number): string {
  const m = Math.max(0, ms)
  if (m < 10 * 1000) return `${(m / 1000).toFixed(1)} sn`
  return formatDuration(m / 1000)
}

// Recency bucket ids, ordered newest → oldest.
// 'pinned' is not a recency bucket — bucketOf never returns it. Callers that
// float pinned rows to the top of a bucketed list assign it themselves.
export type Bucket = 'pinned' | 'today' | 'yesterday' | 'week' | 'month' | 'older'

export const BUCKET_LABELS: Record<Bucket, string> = {
  pinned: 'Sabitlenen',
  today: 'Bugün',
  yesterday: 'Dün',
  week: 'Geçen hafta',
  month: 'Geçen ay',
  older: 'Daha eski',
}

export const BUCKET_ORDER: Bucket[] = ['pinned', 'today', 'yesterday', 'week', 'month', 'older']

// bucketOf classifies a timestamp by calendar day relative to local "today",
// so a chat from 23:30 yesterday lands in "Dün", not "today minus 24h".
export function bucketOf(unixSec: number, nowMs = Date.now()): Bucket {
  const startOfToday = new Date(nowMs)
  startOfToday.setHours(0, 0, 0, 0)
  const ts = unixSec * 1000
  const todayStart = startOfToday.getTime()
  if (ts >= todayStart) return 'today'
  if (ts >= todayStart - DAY * 1000) return 'yesterday'
  if (ts >= todayStart - 7 * DAY * 1000) return 'week'
  if (ts >= todayStart - 30 * DAY * 1000) return 'month'
  return 'older'
}
