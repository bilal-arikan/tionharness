// Search-index lifecycle endpoints (internal/api/search_indexes*.go).
//
// The listing is process-wide rather than workspace-scoped: two workspaces
// opened on the same repository share one store on disk, so a per-workspace view
// would show the same index twice with two different answers. The mutations DO
// go through the active workspace's runtime, which is what actually runs the
// indexer.
import type { SearchIndexStatus, SearchIndexRefreshResult, SearchIndexDropResult } from '@/types'
import { req } from './client'

export const searchIndexApi = {
  listSearchIndexes: () => req<SearchIndexStatus[]>('/api/search-indexes'),

  /**
   * Start a refresh (re-embed in place) or rebuild (discard the store first) of
   * one index. Resolves as soon as the run is CLAIMED, not when it finishes —
   * indexing takes minutes, so the caller must poll the listing for the outcome.
   *
   * Answers 409 when a run is already in flight for the same root.
   */
  refreshSearchIndex: (tool: string, root: string, rebuild = false) =>
    req<SearchIndexRefreshResult>('/api/search-indexes/refresh', {
      method: 'POST',
      body: JSON.stringify({ tool, root, rebuild }),
    }),

  /**
   * Delete one index from disk. `confirmRoot` must repeat `root` exactly or the
   * backend answers 409 and deletes nothing — the confirmation is a path rather
   * than a boolean so that whoever confirmed had to name the specific index
   * being destroyed. Never call this without an explicit user action.
   */
  dropSearchIndex: (tool: string, root: string, confirmRoot: string) =>
    req<SearchIndexDropResult>('/api/search-indexes/drop', {
      method: 'POST',
      body: JSON.stringify({ tool, root, confirmRoot }),
    }),
}
