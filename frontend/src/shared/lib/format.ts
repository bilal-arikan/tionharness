// Shared display formatters used across budget/usage/session panels. Kept in one
// place so the same value renders identically everywhere.
//
// These are locale-SENSITIVE but not word-translated: the digit grouping and
// decimal separator follow the active UI locale (Intl), while the units are the
// symbols and SI-style abbreviations that read the same in every language we
// support. Anything needing an actual translated word belongs in a catalog.

import { numberFormat } from './intl'

// usd renders a USD cost. Sub-cent amounts get more precision so tiny spends
// don't all collapse to $0.00. The currency stays USD regardless of locale —
// this is what the provider bills, not a value to convert — but the number is
// grouped per locale ("$1,234.56" in en-US, "$1.234,56" in tr-TR).
export function usd(n: number): string {
  if (n === 0) return '$0'
  const digits = n !== 0 && Math.abs(n) < 0.01 ? 4 : 2
  return numberFormat({
    style: 'currency',
    currency: 'USD',
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  }).format(n)
}

// count renders a plain integer with locale digit grouping. Replaces the
// scattered `n.toLocaleString('tr-TR')` calls, which hardcoded Turkish grouping
// into every panel.
export function count(n: number): string {
  return numberFormat().format(n)
}

// decimal renders a number with a fixed number of fraction digits, grouped per
// locale. Use instead of toFixed() anywhere the result is shown to a user.
function decimal(n: number, digits = 1): string {
  return numberFormat({ minimumFractionDigits: digits, maximumFractionDigits: digits }).format(n)
}

// tokens renders a token count compactly as "123", "1.2k" or "3.4M".
export function tokens(n: number): string {
  if (n >= 1_000_000) return `${decimal(n / 1_000_000)}M`
  if (n >= 1000) return `${decimal(n / 1000)}k`
  return count(n)
}

// percent renders a 0..1 ratio as a percentage string (e.g. "42%"). Uses Intl's
// percent style so the symbol lands on the side the locale expects — Turkish
// writes "%42", English "42%".
export function percent(ratio: number, digits = 0): string {
  return numberFormat({
    style: 'percent',
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  }).format(ratio)
}
