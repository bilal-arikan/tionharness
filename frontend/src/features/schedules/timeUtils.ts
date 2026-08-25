import { formatDateTime } from '@/shared/lib/intl'
// Shared unix-seconds <-> UI time helpers for the automation board (schedules,
// tag automations and board automations all use the same expiry input).

export function fmtTime(unix?: number): string {
  if (!unix) return '—'
  return formatDateTime(new Date(unix * 1000), { dateStyle: 'short', timeStyle: 'medium' })
}

// Convert a unix-seconds timestamp to the "YYYY-MM-DDTHH:mm" string a
// datetime-local input expects (in local time). 0/undefined → empty string.
export function unixToLocalInput(unix?: number): string {
  if (!unix) return ''
  const d = new Date(unix * 1000)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

// Parse a datetime-local input string back to unix seconds. Empty → 0.
export function localInputToUnix(s: string): number {
  if (!s) return 0
  const ms = new Date(s).getTime()
  return isNaN(ms) ? 0 : Math.floor(ms / 1000)
}

export function isPast(unix?: number): boolean {
  return !!unix && unix <= Math.floor(Date.now() / 1000)
}
