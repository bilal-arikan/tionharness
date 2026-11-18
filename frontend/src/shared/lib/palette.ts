// Categorical colors: stable hues that distinguish *data categories* (call
// kinds, ...). Unlike the theme tokens in index.css, these
// are intentionally fixed — they identify a category rather than express UI
// chrome, so they deliberately do NOT track the light/dark theme. Centralised
// here so the same category reads with the same hue everywhere it appears.

// Neutral slate used when a category is unknown / unmapped.
const CATEGORY_FALLBACK = '#94a3b8'

// Call-origin colors — used by the Budget breakdown (one hue per call kind).
export const KIND_COLORS: Record<string, string> = {
  chat: '#6366f1',
  task: '#0ea5e9',
  schedule: '#14b8a6',
  flow: '#a855f7',
  delegate: '#ec4899',
  title: '#84cc16',
  summary: '#22c55e',
  reflect: '#eab308',
  compact: '#ef4444',
  decide: '#f97316',
  other: CATEGORY_FALLBACK,
}

export function kindColor(kind: string): string {
  return KIND_COLORS[kind] ?? CATEGORY_FALLBACK
}
