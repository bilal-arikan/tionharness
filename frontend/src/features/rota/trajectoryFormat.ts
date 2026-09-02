// Small formatters for trajectory metrics (Rota F3).

export function fmtDurationSec(sec: number): string {
  if (!sec || sec < 0) return '0 sn'
  if (sec < 60) return `${Math.round(sec)} sn`
  const m = Math.floor(sec / 60)
  if (m < 60) return `${m} dk`
  const h = Math.floor(m / 60)
  const rm = m % 60
  if (h < 24) return rm ? `${h} sa ${rm} dk` : `${h} sa`
  const d = Math.floor(h / 24)
  return `${d} g ${h % 24} sa`
}

export function fmtTokens(n: number): string {
  if (!n) return '0'
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(n >= 100_000 ? 0 : 1)}k`
  return String(n)
}
