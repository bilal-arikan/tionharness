import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { api } from '@/api'
import type { ViewGraphResult, ViewRef } from '@/types'
import { parseRef, refToString } from '@/types'
import {
  applyExplorerFilter,
  childCounts,
  emptyExplorerFilter,
  explorerFacets,
  type ExplorerFilter,
} from './explorerFilter'
import { augmentLive, panelRefFor } from './explorerLive'
import { seedLayout } from './explorerSeed'
import { graphToVis, kindColor, resolveExplorerTheme, ROOT_KEY, ROOT_REF } from './explorerVis'
import type { ExplorerBucket } from './ExplorerFilters'

const EMPTY_FILTER = emptyExplorerFilter()
const NO_COLLAPSED: ReadonlySet<string> = new Set()

interface Options {
  onError?: (msg: string) => void
  search: string
  // Facet filter (explorerFilter); omitted = everything visible.
  filter?: ExplorerFilter
  // Nodes folded from the side panel (useExplorerCollapse).
  collapsed?: ReadonlySet<string>
  // Deep link: the node to select + focus on entry (a ref string), and the
  // callback that mirrors every user selection back into the URL.
  initialFocus?: string | null
  onFocus?: (refString: string | null) => void
}

// useExplorerGraph owns the Explorer network's data: one whole-map fetch
// (GET /api/views/graph), the live layer derived from it, the filter, the
// selected node, and the camera focus request the canvas honours. Selection ==
// focus here: a single click both opens the node in the side panel and glides
// the camera to it.
export function useExplorerGraph({
  onError,
  search,
  filter = EMPTY_FILTER,
  collapsed = NO_COLLAPSED,
  initialFocus,
  onFocus,
}: Options) {
  const [graph, setGraph] = useState<ViewGraphResult | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | undefined>()
  const parsedInitial = initialFocus ? parseRef(initialFocus) : null
  const [selectedRef, setSelectedRef] = useState<ViewRef>(() => parsedInitial ?? ROOT_REF)
  const [focus, setFocus] = useState<{ key: string | null; tick: number }>(() => ({
    key: parsedInitial ? refToString(parsedInitial) : null,
    tick: 0,
  }))
  const [themeVersion, setThemeVersion] = useState(0)
  const requestRef = useRef<AbortController | null>(null)
  const sequenceRef = useRef(0)

  const load = useCallback(() => {
    requestRef.current?.abort()
    const controller = new AbortController()
    requestRef.current = controller
    const sequence = ++sequenceRef.current
    setLoading(true)
    api
      .viewGraph(controller.signal)
      .then((result) => {
        if (sequenceRef.current !== sequence) return
        setGraph(result)
        setError(undefined)
      })
      .catch((err: unknown) => {
        if (controller.signal.aborted || sequenceRef.current !== sequence) return
        const message = err instanceof Error ? err.message : String(err)
        setError(message)
        onError?.(message)
      })
      .finally(() => {
        if (sequenceRef.current === sequence) setLoading(false)
      })
  }, [onError])

  useEffect(() => {
    // The fetch is the effect; its loading flag is set synchronously on purpose.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    load()
    return () => requestRef.current?.abort()
  }, [load])

  // URL navigation (back/forward, a pasted link) is external state: mirror it
  // into selection + focus. An unparsable ref falls back to the root and reports.
  const [deepLinkError, setDeepLinkError] = useState<string | undefined>(() =>
    initialFocus && !parsedInitial ? `Geçersiz odak bağlantısı: ${initialFocus}` : undefined,
  )
  useEffect(() => {
    if (!initialFocus) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setDeepLinkError(undefined)
      return
    }
    const next = parseRef(initialFocus)
    if (!next) {
      setDeepLinkError(`Geçersiz odak bağlantısı: ${initialFocus}`)
      setSelectedRef(ROOT_REF)
      return
    }
    setDeepLinkError(undefined)
    const key = refToString(next)
    setSelectedRef((current) => (refToString(current) === key ? current : next))
    setFocus((current) => (current.key === key ? current : { key, tick: current.tick + 1 }))
  }, [initialFocus])

  // Live layer (avatar nodes off executing sessions) over the raw map, then the
  // filter over that. Both are pure and cheap relative to the physics.
  const live = useMemo(() => (graph ? augmentLive(graph) : null), [graph])
  const visible = useMemo(
    () =>
      live ? applyExplorerFilter(live.graph, filter, live.liveState, ROOT_KEY, collapsed) : null,
    [live, filter, collapsed],
  )
  // Children per node on the full (unfiltered) map: the fold toggle's count.
  const counts = useMemo(() => (live ? childCounts(live.graph) : new Map<string, number>()), [live])
  const facets = useMemo(
    () => (graph ? explorerFacets(graph) : { kinds: [], agents: [], tags: [] }),
    [graph],
  )
  // Layer chips: the root's direct children, in the order the backend lists them.
  const buckets = useMemo<ExplorerBucket[]>(() => {
    if (!graph) return []
    const byKey = new Map(graph.nodes.map((h) => [refToString(h.ref), h]))
    return graph.edges
      .filter((e) => refToString(e.source) === ROOT_KEY)
      .map((e) => byKey.get(refToString(e.target)))
      .filter((h): h is NonNullable<typeof h> => !!h)
      .map((h) => ({ key: refToString(h.ref), label: h.label, color: kindColor(h.ref) }))
  }, [graph])

  // A deep-linked node that the loaded map does not contain is an error the user
  // can see (and escape from), not a silent root selection.
  const selectedKey = refToString(selectedRef)
  const refByKey = useMemo(() => {
    const map = new Map<string, ViewRef>()
    for (const handle of live?.graph.nodes ?? []) map.set(refToString(handle.ref), handle.ref)
    return map
  }, [live])
  const missingFocus =
    graph !== null && initialFocus && !deepLinkError && !refByKey.has(selectedKey)
      ? `Odak düğümü haritada yok: ${selectedKey}`
      : undefined

  const select = useCallback(
    (ref: ViewRef) => {
      const key = refToString(ref)
      setSelectedRef(ref)
      setFocus((current) => ({ key, tick: current.tick + 1 }))
      onFocus?.(key === ROOT_KEY ? null : key)
    },
    [onFocus],
  )
  const selectKey = useCallback(
    (key: string) => {
      const ref = refByKey.get(key)
      if (ref) select(ref)
    },
    [refByKey, select],
  )
  const fallbackToRoot = useCallback(() => select(ROOT_REF), [select])

  // Theme presets are applied as inline root tokens; data-theme additionally
  // distinguishes light mode. Node colors are baked into the vis data, so
  // re-map when either changes.
  useEffect(() => {
    const observer = new MutationObserver(() => setThemeVersion((v) => v + 1))
    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['style', 'data-theme'],
    })
    return () => observer.disconnect()
  }, [])

  const layout = useMemo(() => (visible ? seedLayout(visible, ROOT_KEY) : null), [visible])
  const { nodes, edges } = useMemo(() => {
    if (!visible || !layout || !live) return { nodes: [], edges: [] }
    // themeVersion is a re-map trigger, not an input.
    void themeVersion
    return graphToVis(visible, {
      selectedKey,
      search,
      theme: resolveExplorerTheme(),
      layout,
      liveState: live.liveState,
      liveAgents: live.liveAgents,
      collapsed,
      childCounts: counts,
    })
  }, [visible, layout, live, selectedKey, search, themeVersion, collapsed, counts])
  const canonicalNodeIds = useMemo(
    () => (live ? live.graph.nodes.map((handle) => refToString(handle.ref)) : []),
    [live],
  )

  return {
    graph,
    // The map as drawn (live layer + filter applied): search and counts read this.
    visibleGraph: visible,
    facets,
    buckets,
    liveCount: live?.liveState.size ?? 0,
    // How many children the selected node has on the full map (0 = leaf).
    selectedChildCount: counts.get(selectedKey) ?? 0,
    nodes,
    edges,
    canonicalNodeIds,
    ready: graph !== null,
    loading,
    error,
    deepLinkError: deepLinkError ?? missingFocus,
    selectedRef,
    // What the side panel projects: a live avatar node shows its agent's card.
    panelRef: panelRefFor(selectedRef),
    selectedKey,
    focusKey: focus.key,
    focusTick: focus.tick,
    select,
    selectKey,
    fallbackToRoot,
    refresh: load,
  }
}
