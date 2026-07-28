// Categorical colors: stable hues that distinguish *data categories* (call
// kinds, context-bucket roles, ...). Unlike the theme tokens in index.css, these
// are intentionally fixed — they identify a category rather than express UI
// chrome, so they deliberately do NOT track the light/dark theme. Centralised
// here so the same category reads with the same hue everywhere it appears.

// Neutral slate used when a category is unknown / unmapped.
export const CATEGORY_FALLBACK = '#94a3b8'

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
  other: CATEGORY_FALLBACK,
}

export function kindColor(kind: string): string {
  return KIND_COLORS[kind] ?? CATEGORY_FALLBACK
}

// Context-bucket role colors — used by the session context-usage bar + legend.
// `user` intentionally maps to the live accent token so the user's own share
// matches the app accent.
export const ROLE_COLORS: Record<string, string> = {
  summary: '#f59e0b', // amber — folded history
  user: 'var(--color-accent)',
  assistant: '#10b981', // emerald
  tool: '#8b5cf6', // violet — tool-result messages
  tools: '#a855f7', // purple — tool/MCP schemas (always-sent catalog)
  artifacts: '#ec4899', // pink — session artifact context block
  system: '#64748b', // slate
}

export function roleColor(role: string): string {
  return ROLE_COLORS[role] ?? CATEGORY_FALLBACK
}
