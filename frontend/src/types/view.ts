// Projection layer types — mirrors internal/view (see _Docs/66-VIEW-KATMANI.md).
//
// A view is the compact, deterministic summary of a large entity. The SAME bytes
// go to the agent and to this UI: the panel renders `text` verbatim instead of
// re-composing it from header/body, so a wrong or stale projection is visible to
// the user rather than hidden behind a prettier rendering.

export type ViewKind = 'flowrun' | 'session' | 'board' | 'workspace' | 'schedule'

// Budget tiers. tiny is one dense line (safe to push into a prompt suffix), card
// is the default, full adds per-item detail.
export type ViewLevel = 'tiny' | 'card' | 'full'

// Which facts are interesting. Deliberately few — a long menu makes both the
// agent and the user pick badly.
export type ViewLens = 'health' | 'stale' | 'recent' | 'errors'

export interface ViewRef {
  kind: ViewKind
  id: string
  sub?: string
}

// A drill-down pointer: the projection stayed small, and this is how to open the
// part it left out.
export interface ViewHandle {
  label: string
  ref: ViewRef
  level?: ViewLevel
}

export interface ViewResult {
  ref: ViewRef
  level: ViewLevel
  lens: ViewLens
  header: string
  body: string
  // The rendered projection exactly as an agent receives it.
  text: string
  handles: ViewHandle[] | null
  asOf: string
  // The revision projected (status/updatedAt/trace length) — two views with the
  // same source describe the same state.
  source: string
  // How many items the projection deliberately hid. Always rendered.
  elided: number
  // What was hidden ("kart", "eski mesaj", "node"). A bare count is ambiguous —
  // 174 hidden messages and 174 hidden cards mean very different things.
  elidedUnit?: string
  // Approximate token cost (chars/4) of header+body.
  tokens: number
}

// refToString spells a ref the way handles and the get_view tool do.
export function refToString(ref: ViewRef): string {
  return `${ref.kind}:${ref.id}${ref.sub ? `#${ref.sub}` : ''}`
}

export const VIEW_LENS_LABEL: Record<ViewLens, string> = {
  health: 'sağlık',
  stale: 'duran',
  recent: 'son değişim',
  errors: 'hatalar',
}
