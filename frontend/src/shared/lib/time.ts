// Time helpers for the session list: relative "updated" labels, durations and
// coarse recency buckets (today / yesterday / last week / last month / older).
//
// These are localized in two independent ways and both matter:
//   - the WORDS come from the `common:time.*` catalog (i18next `t`, read at call
//     time so a language switch is picked up without re-importing), and
//   - the NUMBERS and DATES come from Intl against the active locale tag, so a
//     Turkish user keeps a 24-hour clock and dotted thousands while an English
//     one gets the en-US conventions.
//
// The module-level `t` is i18next's global translator rather than the React hook
// because these functions are called from plain helpers and sort comparators, not
// only from components. Locale changes force a root remount (see I18nRoot), so a
// value computed outside React still cannot go stale on screen.

import { i18next } from '@/i18n'
import { dateFormat } from './intl'

const MIN = 60
const HOUR = 60 * MIN
const DAY = 24 * HOUR

function t(key: string, params?: Record<string, unknown>): string {
  return i18next.t(key, { ns: 'common', ...params }) as string
}

// relativeTime formats a unix-seconds timestamp as a short "updated" label
// (e.g. "az önce" / "just now", "5 dk", "3 sa", "Dün", "12 Haz"). Anything older
// than a week falls back to a day-month date in the active locale.
export function relativeTime(unixSec: number, nowMs = Date.now()): string {
  if (!unixSec) return ''
  const diff = Math.max(0, Math.floor(nowMs / 1000) - unixSec)
  if (diff < MIN) return t('time.justNow')
  if (diff < HOUR) return t('time.minutesShort', { count: Math.floor(diff / MIN) })
  if (diff < DAY) return t('time.hoursShort', { count: Math.floor(diff / HOUR) })
  if (diff < 2 * DAY) return t('time.yesterday')
  if (diff < 7 * DAY) return t('time.daysShort', { count: Math.floor(diff / DAY) })
  // Older: show a day-month date.
  return dateFormat({ day: 'numeric', month: 'short' }).format(new Date(unixSec * 1000))
}

// clockTime formats a unix-seconds timestamp as a short local clock ("22:43").
export function clockTime(unixSec: number): string {
  if (!unixSec) return ''
  return dateFormat({ hour: '2-digit', minute: '2-digit' }).format(new Date(unixSec * 1000))
}

// fullDateTime formats a unix-seconds timestamp as a full local date+time, used
// as a hover title alongside the short clock label.
export function fullDateTime(unixSec: number): string {
  if (!unixSec) return ''
  return dateFormat({
    day: 'numeric',
    month: 'long',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).format(new Date(unixSec * 1000))
}

// formatDuration renders a span of seconds as a compact label ("45 sn",
// "2 dk 15 sn", "1 sa 5 dk"). Used for how long an agent turn took and for the
// live elapsed counter of an in-flight turn.
export function formatDuration(seconds: number): string {
  const s = Math.max(0, Math.floor(seconds))
  if (s < MIN) return t('time.duration.seconds', { count: s })
  if (s < HOUR) {
    const m = Math.floor(s / MIN)
    const rem = s % MIN
    return rem
      ? t('time.duration.minutesSeconds', { minutes: m, seconds: rem })
      : t('time.duration.minutes', { count: m })
  }
  const h = Math.floor(s / HOUR)
  const m = Math.floor((s % HOUR) / MIN)
  return m
    ? t('time.duration.hoursMinutes', { hours: h, minutes: m })
    : t('time.duration.hours', { count: h })
}

// formatDurationMs renders a server-measured duration (milliseconds) as a
// compact label. Short turns keep one decimal ("0.8 sn", "3.4 sn") because the
// backend measures in ms and flooring them to whole seconds would throw the
// precision away; from 10 s up it falls back to formatDuration.
export function formatDurationMs(ms: number): string {
  const m = Math.max(0, ms)
  if (m < 10 * 1000) return t('time.duration.secondsDecimal', { value: (m / 1000).toFixed(1) })
  return formatDuration(m / 1000)
}

// Recency bucket ids, ordered newest → oldest.
// 'pinned' is not a recency bucket — bucketOf never returns it. Callers that
// float pinned rows to the top of a bucketed list assign it themselves.
export type Bucket = 'pinned' | 'today' | 'yesterday' | 'week' | 'month' | 'older'

export const BUCKET_ORDER: Bucket[] = ['pinned', 'today', 'yesterday', 'week', 'month', 'older']

// bucketLabel is the translated heading for a recency bucket. This replaced a
// BUCKET_LABELS constant map: a module-level object would freeze the labels in
// whatever language was active when the module first loaded.
export function bucketLabel(bucket: Bucket): string {
  return t(`time.bucket.${bucket}`)
}

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
