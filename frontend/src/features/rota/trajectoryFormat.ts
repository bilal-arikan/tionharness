// Small formatters for trajectory metrics (Rota F3).
import { i18next } from '@/i18n'

export function fmtDurationSec(sec: number): string {
  if (!sec || sec < 0) return i18next.t('duration.seconds', { ns: 'rota', count: 0 })
  if (sec < 60) return i18next.t('duration.seconds', { ns: 'rota', count: Math.round(sec) })
  const m = Math.floor(sec / 60)
  if (m < 60) return i18next.t('duration.minutes', { ns: 'rota', count: m })
  const h = Math.floor(m / 60)
  const rm = m % 60
  if (h < 24)
    return rm
      ? i18next.t('duration.hoursMinutes', { ns: 'rota', hours: h, minutes: rm })
      : i18next.t('duration.hours', { ns: 'rota', count: h })
  const d = Math.floor(h / 24)
  return i18next.t('duration.daysHoursShort', { ns: 'rota', days: d, hours: h % 24 })
}

export function fmtTokens(n: number): string {
  if (!n) return '0'
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(n >= 100_000 ? 0 : 1)}k`
  return String(n)
}
