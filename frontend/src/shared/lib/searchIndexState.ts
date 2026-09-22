// Presentation rules for search-index rows, kept out of the component so the
// decisions that matter — which phase is alarming, when to keep polling, which
// actions are legal — are testable without a DOM.
import type { SearchIndexPhase, SearchIndexStatus } from '@/types'

/** How a phase is rendered: label, and the CSS variable carrying its colour. */
export interface PhaseLook {
  label: string
  /** A `var(--color-…)` token, applied as text/border colour by the badge. */
  tone: string
}

/**
 * Per-phase presentation. `failed` and `missing` deliberately do NOT share a
 * tone with `ready`: an index that cannot answer must never look like one that
 * can, which is the same reason the backend reports `usable` rather than
 * letting the client infer it.
 */
const PHASE_LOOK: Record<SearchIndexPhase, PhaseLook> = {
  ready: { label: 'hazır', tone: 'var(--color-success)' },
  indexing: { label: 'indeksleniyor…', tone: 'var(--color-accent)' },
  stale: { label: 'bayat', tone: 'var(--color-warning)' },
  failed: { label: 'başarısız', tone: 'var(--color-danger)' },
  missing: { label: 'yok', tone: 'var(--color-text-dim)' },
}

/**
 * Look for a phase. An unrecognised phase falls back to a neutral badge showing
 * the RAW value rather than pretending it is one of the known ones — a backend
 * that grew a phase should show up as unknown, not silently as "ready".
 */
export function phaseLook(phase: string): PhaseLook {
  return PHASE_LOOK[phase as SearchIndexPhase] ?? { label: phase, tone: 'var(--color-text-dim)' }
}

/**
 * Whether any listed index has a run in flight, i.e. whether the list is still
 * moving and must be re-fetched. This is the poll condition: indexing is the
 * only phase that changes on its own.
 */
export function isAnyIndexing(rows: SearchIndexStatus[]): boolean {
  return rows.some((r) => r.phase === 'indexing')
}

/**
 * Whether refresh/rebuild/drop may be offered for a row.
 *
 * A run already in flight owns the entry — the backend answers 409 for a second
 * one — so the buttons are disabled rather than left clickable to produce an
 * error the user cannot act on.
 */
export function canActOn(row: SearchIndexStatus): boolean {
  return row.phase !== 'indexing'
}

/**
 * A refresh of an index that is not on disk yet is a create, which is how the
 * backend resolves it too. Naming that here keeps the button's title honest
 * instead of promising a re-embed of nothing.
 */
export function refreshVerb(row: SearchIndexStatus): 'Refresh' | 'Create' {
  return row.phase === 'missing' ? 'Create' : 'Refresh'
}

/**
 * Format an RFC3339 timestamp as a short local date-time for the "last updated"
 * column. An absent or unparseable value yields an em dash rather than
 * "Invalid Date": the row is still meaningful without it.
 */
export function formatIndexTime(value: string | undefined, locale?: string): string {
  if (!value) return '—'
  const ms = Date.parse(value)
  if (Number.isNaN(ms)) return '—'
  return new Date(ms).toLocaleString(locale, {
    day: 'numeric',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
  })
}
