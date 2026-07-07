// Shared display formatters used across budget/usage/session panels. Kept in one
// place so the same value renders identically everywhere. These are pure and
// side-effect free.

// usd renders a USD cost. Sub-cent amounts get more precision so tiny spends
// don't all collapse to $0.00.
export function usd(n: number): string {
  if (n === 0) return '$0'
  if (n < 0.01) return `$${n.toFixed(4)}`
  return `$${n.toFixed(2)}`
}

// tokens renders a token count compactly as "123", "1.2k" or "3.4M".
export function tokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1000) return `${(n / 1000).toFixed(1)}k`
  return `${n}`
}

// Backwards-compatible alias for the token formatter (some panels imported it
// as fmtTokens / fmt).
export const fmtTokens = tokens

// bytes renders a byte count as B/KB/MB/GB. Used for compaction savings meters
// which are tracked in raw bytes, not tokens.
export function bytes(n: number): string {
  if (n < 1024) return `${n} B`
  const units = ['KB', 'MB', 'GB']
  let v = n / 1024
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(v < 10 ? 1 : 0)} ${units[i]}`
}

// approxTokens estimates tokens from bytes at the plain-text ~4 chars/token
// ratio. Honest approximation for compaction savings; shown as "~N token".
export function approxTokens(n: number): number {
  return Math.round(n / 4)
}

// percent renders a 0..1 ratio as an integer percentage string (e.g. "42%").
export function percent(ratio: number, digits = 0): string {
  return `${(ratio * 100).toFixed(digits)}%`
}
