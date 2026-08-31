import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { Dispatch, SetStateAction } from 'react'
import { api } from '@/api'
import type { Artifact } from '@/types'
import { useSessionState } from '@/shared/hooks/useSessionState'
import { useDebouncedValue } from '@/shared/hooks/useDebouncedValue'
import { useGroupedList } from '@/shared/hooks/useGroupedList'
import { compareText } from '@/shared/lib/intl'
import {
  ARTIFACTS_PAGE_SIZE,
  artifactGroupKey,
  sortArtifactGroups,
  type OriginFilter,
} from './artifactGrouping'

export interface ArtifactList {
  list: Artifact[]
  setList: Dispatch<SetStateAction<Artifact[]>>
  activeId: string | null
  setActiveId: Dispatch<SetStateAction<string | null>>
  // Filters (all applied server-side, so the loaded page(s) ARE the rendered list).
  query: string
  setQuery: Dispatch<SetStateAction<string>>
  originFilter: OriginFilter
  setOriginFilter: Dispatch<SetStateAction<OriginFilter>>
  showArchived: boolean
  setShowArchived: Dispatch<SetStateAction<boolean>>
  hasActiveFilters: boolean
  // Paging + loading state.
  loading: boolean
  total: number
  hasMore: boolean
  loadingMore: boolean
  archivedCount: number
  reload: () => void
  loadMore: () => void
  // Grouped view of the loaded list.
  grouped: Array<[string, Artifact[]]>
  collapsed: Set<string>
  toggleGroup: (name: string) => void
  allCollapsed: boolean
  toggleAll: () => void
  // Distinct existing group names, offered as autocomplete suggestions.
  groupNames: string[]
  // Flattened visible (non-collapsed) id order — feeds multi-select + select-all.
  orderedIds: string[]
}

// useArtifactList owns the artifacts list itself: the server-side filters, the
// limit/offset paging (same contract as the list_artifacts tool), the grouped
// view built on top of it, and the current selection id.
export function useArtifactList(
  selectedId: string | null | undefined,
  artifactsTick: unknown,
  onError: (msg: string) => void,
): ArtifactList {
  const [list, setList] = useState<Artifact[]>([])
  // Selection persists across screen switches within the session (resets on app
  // reload). A deep-link `selectedId` still overrides via the effect below.
  const [activeId, setActiveId] = useSessionState<string | null>(
    'artifacts.activeId',
    selectedId ?? null,
  )
  // List filters: free-text title search + an origin facet (Tümü / chat / manual /
  // agent / tool).
  const [query, setQuery] = useState('')
  const [originFilter, setOriginFilter] = useState<OriginFilter>('all')
  // Archived view toggle: false (default) hides archived artifacts and shows only
  // active ones; true flips to show ONLY archived artifacts (so they can be
  // reviewed and un-archived). Persisted so switching screens keeps the view.
  const [showArchived, setShowArchived] = useSessionState<boolean>('artifacts.showArchived', false)
  // True until the first artifact list lands — the list column shows a loading
  // state rather than the "no artifacts yet" onboarding copy.
  const [loading, setLoading] = useState(true)

  // Server-side paging (same limit/offset contract as the list_artifacts tool):
  // the panel loads ARTIFACTS_PAGE_SIZE rows at a time and appends on "load
  // more". Filters (title search, origin facet, archived view) run server-side
  // so paged results stay correct; total/hasMore drive the counter and button.
  const [total, setTotal] = useState(0)
  const [hasMore, setHasMore] = useState(false)
  const [loadingMore, setLoadingMore] = useState(false)
  // Count of archived artifacts across the whole store — drives the archived
  // toggle's badge and hides the toggle entirely until something is archived.
  const [archivedTotal, setArchivedTotal] = useState(0)
  const archivedCount = archivedTotal
  const debouncedQuery = useDebouncedValue(query, 300)
  const hasActiveFilters = query.trim() !== '' || originFilter !== 'all' || showArchived

  // Sequence number of the newest list request. Filters change fast (300 ms
  // search debounce, origin facet, archive toggle), so a slow earlier response
  // must not overwrite the list a newer request already produced.
  const listReq = useRef(0)

  const fetchPage = useCallback(
    async (offset: number, append: boolean) => {
      const seq = ++listReq.current
      const origin = originFilter === 'all' ? undefined : originFilter
      const r = await api.listArtifacts({
        limit: ARTIFACTS_PAGE_SIZE,
        offset,
        q: debouncedQuery.trim() || undefined,
        origin,
        archived: showArchived,
      })
      // Superseded while in flight — drop the stale page.
      if (seq !== listReq.current) return
      setList((prev) => (append ? [...prev, ...r.items] : r.items))
      setTotal(r.total)
      setHasMore(r.hasMore)
      if (!append) setActiveId((cur) => cur ?? r.items[0]?.id ?? null)
    },
    [originFilter, debouncedQuery, showArchived],
  )

  const reload = useCallback(() => {
    setLoading(true)
    fetchPage(0, false)
      .catch((e) => onError((e as Error).message))
      .finally(() => setLoading(false))
    // Archive badge count — only meaningful from the active view (the archived
    // view already knows its own total).
    if (!showArchived) {
      api
        .listArtifacts({ archived: true, limit: 1 })
        .then((r) => setArchivedTotal(r.total))
        .catch(() => {})
    }
  }, [fetchPage, showArchived, onError])

  const loadMore = useCallback(() => {
    if (loadingMore || !hasMore) return
    setLoadingMore(true)
    fetchPage(list.length, true)
      .catch((e) => onError((e as Error).message))
      .finally(() => setLoadingMore(false))
  }, [fetchPage, loadingMore, hasMore, list.length, onError])

  useEffect(() => {
    reload()
    // Invalidate whatever is still in flight when the filters change or the
    // panel unmounts, so a late page can't land on the next filter's list.
    return () => {
      listReq.current += 1
    }
  }, [reload, artifactsTick])

  // Once nothing is archived any more (e.g. the last archived artifact was
  // restored), fall back to the active view so the archived view can't strand
  // the user on a permanently empty list.
  useEffect(() => {
    if (showArchived && archivedCount === 0) setShowArchived(false)
  }, [showArchived, archivedCount, setShowArchived])

  // Honour an incoming deep-link selection (e.g. clicking an artifact card).
  useEffect(() => {
    if (selectedId) setActiveId(selectedId)
  }, [selectedId])

  // Filtered artifacts bucketed by group (ungrouped first, then named groups),
  // with persisted per-group collapse state. Mirrors the Skills screen so both
  // list screens organise the same way. Grouping runs on the already-filtered
  // list so search/origin facets still apply.
  const {
    groups: grouped,
    collapsed,
    toggle: toggleGroup,
    allCollapsed,
    toggleAll,
  } = useGroupedList(list, {
    keyOf: artifactGroupKey,
    sortGroups: sortArtifactGroups,
    persistKey: 'tionharness.artifactsCollapsedGroups',
  })
  // Distinct existing group names (across the full list, not just the filtered
  // view), offered as bulk-group autocomplete suggestions.
  const groupNames = useMemo(
    () =>
      [...new Set(list.map((a) => a.group?.trim()).filter((g): g is string => !!g))].sort((a, b) =>
        compareText(a, b),
      ),
    [list],
  )
  // Flattened visible (non-collapsed) id order, so a Shift+Click range can cross
  // group boundaries but skips folded groups. Feeds multi-select + select-all.
  const orderedIds = useMemo(
    () => grouped.flatMap(([name, items]) => (collapsed.has(name) ? [] : items.map((a) => a.id))),
    [grouped, collapsed],
  )

  return {
    list,
    setList,
    activeId,
    setActiveId,
    query,
    setQuery,
    originFilter,
    setOriginFilter,
    showArchived,
    setShowArchived,
    hasActiveFilters,
    loading,
    total,
    hasMore,
    loadingMore,
    archivedCount,
    reload,
    loadMore,
    grouped,
    collapsed,
    toggleGroup,
    allCollapsed,
    toggleAll,
    groupNames,
    orderedIds,
  }
}
