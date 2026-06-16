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

// Recency bucket ids, ordered newest → oldest.
export type Bucket = 'today' | 'yesterday' | 'week' | 'month' | 'older'

export const BUCKET_LABELS: Record<Bucket, string> = {
  today: 'Bugün',
  yesterday: 'Dün',
  week: 'Geçen hafta',
  month: 'Geçen ay',
  older: 'Daha eski',
}

export const BUCKET_ORDER: Bucket[] = ['today', 'yesterday', 'week', 'month', 'older']

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
