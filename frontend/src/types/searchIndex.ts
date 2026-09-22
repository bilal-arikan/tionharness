// Search-index lifecycle, keyed by (tool, root) — the UI mirror of
// internal/indexstate. Covers the vector/graph stores TionHarness builds on the
// user's behalf (zvec-grep, codebase-memory).

/**
 * Where one index sits in its lifecycle. The set is closed and mirrors
 * indexstate.Phase exactly; a phase the backend does not send is a bug, not a
 * case to invent client-side.
 *
 * `failed` never decays back to `ready` — that distinction is the reason the
 * backend tracks a phase instead of a boolean, so the UI must not collapse it.
 */
export type SearchIndexPhase = 'missing' | 'indexing' | 'ready' | 'stale' | 'failed'

/** The run that produced (or is producing) a phase. */
type SearchIndexAction = 'create' | 'refresh' | 'rebuild' | 'drop'

/** One index's recorded state, as served by GET /api/search-indexes. */
export interface SearchIndexStatus {
  tool: string
  root: string
  phase: SearchIndexPhase
  /** The run behind the current phase; empty before the first run. */
  action?: SearchIndexAction
  /** Model the existing store was built with, e.g. `local/potion-code-16m-v2`. */
  embedding?: string
  /** Indexing tool version at build time. */
  toolVersion?: string
  /** Why the last run failed. Only set with phase `failed`. */
  error?: string
  /** RFC3339 UTC bounds of the last run; absent before the first one. */
  startedAt?: string
  updatedAt?: string
  /**
   * Whether a search against this index would answer at all — `ready` or
   * `stale`, never `failed` or `missing`. Server-computed on purpose: deriving
   * it in the UI would let a broken store read as working.
   */
  usable: boolean
}

/** Response of POST /api/search-indexes/refresh. */
export interface SearchIndexRefreshResult {
  tool: string
  root: string
  /**
   * What actually started, which is NOT always what was asked for: a refresh of
   * an index that does not exist yet resolves to a `create`.
   */
  action: SearchIndexAction
  started: boolean
}

/** Response of POST /api/search-indexes/drop. */
export interface SearchIndexDropResult {
  tool: string
  root: string
  dropped: boolean
}
