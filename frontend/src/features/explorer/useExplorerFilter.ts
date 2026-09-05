import { useCallback, useEffect, useState } from 'react'
import {
  EXPLORER_FILTER_KEY,
  emptyExplorerFilter,
  parseExplorerFilter,
  serializeExplorerFilter,
  type ExplorerFilter,
} from './explorerFilter'

// The map's filter, persisted per browser so leaving the screen keeps the
// layers and facets the user set (same pattern as useStoredDensity).
export function useExplorerFilter(): [
  ExplorerFilter,
  (next: ExplorerFilter | ((current: ExplorerFilter) => ExplorerFilter)) => void,
  () => void,
] {
  const [filter, setFilter] = useState<ExplorerFilter>(() => {
    try {
      return parseExplorerFilter(globalThis.localStorage?.getItem(EXPLORER_FILTER_KEY))
    } catch {
      return emptyExplorerFilter()
    }
  })
  useEffect(() => {
    try {
      globalThis.localStorage?.setItem(EXPLORER_FILTER_KEY, serializeExplorerFilter(filter))
    } catch {
      // Persistence is best-effort when storage is blocked by browser policy.
    }
  }, [filter])
  const clear = useCallback(() => setFilter(emptyExplorerFilter()), [])
  return [filter, setFilter, clear]
}
