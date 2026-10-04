import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
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
import { changedKeys, mergeLive, pruneFlashes } from './explorerAttention'
import { augmentLive, panelRefFor } from './explorerLive'
import { seedLayout } from './explorerSeed'
import {
  displayLabel,
  graphToVis,
  kindColor,
  resolveExplorerTheme,
  ROOT_KEY,
  ROOT_REF,
} from './explorerVis'
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
// (GET /api/views/graph), the light live re-pull (GET /api/views/graph/live:
// glows, attention rings and the status strip, nodes untouched), the live
// layer derived from it, the filter, the selected node, the spotlight set, and
// the camera focus request the canvas honours. Selection == focus here: a
// single click both opens the node in the side panel and glides the camera to
// it.
export function useExplorerGraph({
  onError,
  search,
  filter = EMPTY_FILTER,
  collapsed = NO_COLLAPSED,
  initialFocus,
  onFocus,
}: Options) {
  const { t } = useTranslation('explorer')
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
  const liveRequestRef = useRef<AbortController | null>(null)
  const sequenceRef = useRef(0)
  // The last landed payload, for diffing a new one against it (the spotlight)
  // and for laying a live re-pull over it. Written only in callbacks.
  const graphRef = useRef<ViewGraphResult | null>(null)
  // Spotlight: node key -> when its ring or glow last changed.
  const [flashes, setFlashes] = useState<Map<string, number>>(() => new Map())

  // landGraph commits a payload and spotlights what changed since the last one.
  const landGraph = useCallback((next: ViewGraphResult) => {
    const changed = changedKeys(graphRef.current, next)
    graphRef.current = next
    setGraph(next)
    if (changed.length === 0) return
    setFlashes((current) => {
      const merged = new Map(current)
      const now = Date.now()
      for (const key of changed) merged.set(key, now)
      return merged
    })
  }, [])

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
        landGraph(result)
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
  }, [onError, landGraph])

  // refreshLive re-pulls the volatile layers only. Nothing to lay it over until
  // the first whole-map fetch landed. A failed pull keeps the last layer: the
  // next event or the next full refresh repairs it, and a toast per transient
  // error would be noise on a screen that updates on every turn.
  const refreshLive = useCallback(() => {
    if (!graphRef.current) return
    liveRequestRef.current?.abort()
    const controller = new AbortController()
    liveRequestRef.current = controller
    api
      .viewGraphLive(controller.signal)
      .then((live) => {
        if (controller.signal.aborted) return
        const current = graphRef.current
        if (!current) return
        landGraph(mergeLive(current, live))
      })
      .catch(() => {})
  }, [landGraph])

  useEffect(() => {
    // The fetch is the effect; its loading flag is set synchronously on purpose.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    load()
    return () => {
      requestRef.current?.abort()
      liveRequestRef.current?.abort()
    }
  }, [load])

  // The spotlight fades on its own: while anything is lit, prune once a second.
  const spotlightOn = flashes.size > 0
  useEffect(() => {
    if (!spotlightOn) return
    const timer = window.setInterval(
      () => setFlashes((current) => pruneFlashes(current, Date.now())),
      1000,
    )
    return () => window.clearInterval(timer)
  }, [spotlightOn])
  const flashing = useMemo(() => new Set(flashes.keys()), [flashes])
  const attention = useMemo(() => new Map(Object.entries(graph?.attention ?? {})), [graph])

  // URL navigation (back/forward, a pasted link) is external state: mirror it
  // into selection + focus. An unparsable ref falls back to the root and reports.
  const [deepLinkError, setDeepLinkError] = useState<string | undefined>(() =>
    initialFocus && !parsedInitial ? t('errors.invalidFocus', { focus: initialFocus }) : undefined,
  )
  useEffect(() => {
    if (!initialFocus) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setDeepLinkError(undefined)
      return
    }
    const next = parseRef(initialFocus)
    if (!next) {
      setDeepLinkError(t('errors.invalidFocus', { focus: initialFocus }))
      setSelectedRef(ROOT_REF)
      return
    }
    setDeepLinkError(undefined)
    const key = refToString(next)
    setSelectedRef((current) => (refToString(current) === key ? current : next))
    setFocus((current) => (current.key === key ? current : { key, tick: current.tick + 1 }))
  }, [initialFocus, t])

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
      .map((h) => ({ key: refToString(h.ref), label: displayLabel(h), color: kindColor(h.ref) }))
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
      ? t('errors.missingFocus', { focus: selectedKey })
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
      attention,
      flashing,
    })
  }, [
    visible,
    layout,
    live,
    selectedKey,
    search,
    themeVersion,
    collapsed,
    counts,
    attention,
    flashing,
  ])
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
    // Attention layer: the status strip's counters and the per-node rings.
    status: graph?.status ?? null,
    attention,
    flashing,
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
    refreshLive,
  }
}
