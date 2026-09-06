import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  ChevronsDownUp,
  ChevronsUpDown,
  ExternalLink,
  Map as MapIcon,
  MessageSquare,
  RefreshCw,
} from 'lucide-react'
import { useRefreshTrigger } from '@/shared/hooks/useRefreshTrigger'
import { useIsMobile } from '@/shared/hooks/useMediaQuery'
import { refToString } from '@/types'
import { ViewPanel } from '@/features/view/ViewPanel'
import { VisNetworkGraph } from '@/features/network/VisNetworkGraph'
import { useStoredDensity } from '@/features/network/useStoredDensity'
import { ExplorerDetailDrawer } from './ExplorerDetailDrawer'
import { ExplorerFilters } from './ExplorerFilters'
import { useExplorerCollapse } from './useExplorerCollapse'
import { useExplorerFilter } from './useExplorerFilter'
import { screenForRef, type ExplorerTarget } from './explorerNavigation'
import { ExplorerSearch, type ExplorerSearchResult } from './ExplorerSearch'
import { displayLabel, nodeLabelOfKind, ROOT_KEY } from './explorerVis'
import { useExplorerGraph } from './useExplorerGraph'

interface Props {
  workspaceId: string
  onError: (msg: string) => void
  // Open a session transcript (wired by App to setView('chat') + selectSession):
  // a double click on a session node jumps to its conversation.
  onOpenSession?: (sessionId: string) => void
  // Open the screen that owns the selected node (the side panel's top button).
  onOpenTarget?: (target: ExplorerTarget) => void
  // Deep-link: the selected node's ref string, restored from the URL on entry and
  // reported back on every selection so the map is shareable/restorable.
  focusNode?: string | null
  onFocusNode?: (refString: string | null) => void
}

// ExplorerView is the "Harita" screen: the whole workspace as one force-directed
// network (the same vis-network canvas the Ağ screen uses). The workspace root
// sits in the middle, its eleven groups ring it, and every member hangs off its
// group — a click selects a node, glides the camera to it and opens its
// projection in the side panel.
//
// Deliberately SEPARATE from the Network screen: that is a relationship graph of
// running agent instances, this is the structural state drill-down.
export function ExplorerView({
  workspaceId,
  onError,
  onOpenSession,
  onOpenTarget,
  focusNode,
  onFocusNode,
}: Props) {
  const [search, setSearch] = useState('')
  // Persisted per browser: leaving the screen must not reset the packing.
  const [density, setDensity] = useStoredDensity('tionharness.explorerDensity')
  const [filter, setFilter, clearFilter] = useExplorerFilter()
  const fold = useExplorerCollapse(workspaceId)
  const [detailOpen, setDetailOpen] = useState(false)
  const isMobile = useIsMobile()
  const {
    graph,
    nodes,
    edges,
    canonicalNodeIds,
    ready,
    loading,
    error,
    deepLinkError,
    panelRef,
    visibleGraph,
    facets,
    buckets,
    liveCount,
    selectedChildCount,
    selectedKey,
    focusKey,
    focusTick,
    selectKey,
    fallbackToRoot,
    refresh,
  } = useExplorerGraph({
    onError,
    search,
    filter,
    collapsed: fold.collapsed,
    initialFocus: focusNode,
    onFocus: onFocusNode,
  })

  const isNarrowScreen = () =>
    typeof window.matchMedia === 'function' && window.matchMedia('(max-width: 1023px)').matches

  // vis fires selectNode with the node id (= ref string). Deselecting by clicking
  // empty canvas keeps the current selection: the side panel should not blank out.
  const handleSelect = useCallback(
    (id: string | null) => {
      if (!id) return
      selectKey(id)
      if (isNarrowScreen()) setDetailOpen(true)
    },
    [selectKey],
  )
  const handleDoubleClick = useCallback(
    (id: string) => {
      if (id.startsWith('session:') && onOpenSession) onOpenSession(id.slice('session:'.length))
    },
    [onOpenSession],
  )
  const closeDetail = useCallback(() => setDetailOpen(false), [])

  // Live update: the central SSE dispatcher bumps 'explorer' on every structural
  // change; re-pull the whole map (one call) and let the physics absorb the diff.
  const tick = useRefreshTrigger('explorer')
  useEffect(() => {
    if (tick > 0) refresh()
  }, [tick, refresh])

  const searchResults = useMemo<ExplorerSearchResult[]>(() => {
    const needle = search.trim().toLowerCase()
    if (!needle || !visibleGraph) return []
    return visibleGraph.nodes.flatMap((handle) => {
      const key = refToString(handle.ref)
      const label = key === ROOT_KEY ? 'Workspace' : displayLabel(handle)
      if (!label.toLowerCase().includes(needle) && !key.toLowerCase().includes(needle)) return []
      return [{ key, label, kindLabel: nodeLabelOfKind(handle.ref), selected: key === selectedKey }]
    })
  }, [visibleGraph, search, selectedKey])

  const selectedHandle = visibleGraph?.nodes.find(
    (handle) => refToString(handle.ref) === selectedKey,
  )
  const selectionAnnouncement = selectedHandle
    ? `Seçili düğüm ${displayLabel(selectedHandle)} (${nodeLabelOfKind(selectedHandle.ref)}).`
    : ''

  // Top of the side panel: jump to the screen that owns the selected node. A
  // session goes to its transcript, an agent to its card, a card to the board…
  const selectedTarget = onOpenTarget ? screenForRef(panelRef) : null
  // Fold toggle: hides / shows the selected node's subtree on the canvas. Only
  // offered for nodes that have children on the full map.
  const selectedCollapsed = fold.isCollapsed(selectedKey)
  const foldButton = selectedChildCount > 0 && (
    <button
      onClick={() => fold.toggle(selectedKey)}
      aria-pressed={selectedCollapsed}
      className="flex w-full items-center justify-center gap-2 rounded-lg border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
    >
      {selectedCollapsed ? <ChevronsUpDown size={14} /> : <ChevronsDownUp size={14} />}
      <span className="truncate">
        {selectedCollapsed
          ? `Alt düğümleri göster · ${selectedChildCount}`
          : `Alt düğümleri gizle · ${selectedChildCount}`}
      </span>
    </button>
  )
  const openSessionButton = (selectedTarget || foldButton) && (
    <div className="flex flex-col gap-2 border-b border-[var(--color-border)] px-3 py-2">
      {selectedTarget && (
        <button
          onClick={() => onOpenTarget?.(selectedTarget)}
          className="flex w-full items-center justify-center gap-2 rounded-lg bg-[var(--color-accent)] px-3 py-2 text-xs font-semibold text-[var(--color-on-accent)] shadow-sm transition hover:brightness-110 active:brightness-95"
        >
          {selectedTarget.view === 'chat' ? (
            <MessageSquare size={14} />
          ) : (
            <ExternalLink size={14} />
          )}
          <span className="truncate">
            {selectedTarget.label} ekranında aç{selectedTarget.id ? ` · ${selectedTarget.id}` : ''}
          </span>
        </button>
      )}
      {foldButton}
    </div>
  )

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <header className="flex items-center gap-3 border-b border-[var(--color-border)] py-3 max-md:px-3 md:px-6">
        <MapIcon size={16} className="shrink-0 text-[var(--color-accent)]" />
        <span className="shrink-0 text-sm font-semibold">Harita</span>

        <ExplorerSearch
          value={search}
          onChange={setSearch}
          results={searchResults}
          onPick={(key) => {
            selectKey(key)
            if (isNarrowScreen()) setDetailOpen(true)
          }}
        />

        <div className="ml-auto flex items-center gap-3 text-xs">
          {graph && (
            <span className="hidden truncate text-[var(--color-text-dim)] md:inline">
              {graph.nodes.length} düğüm · {graph.edges.length} bağlantı
            </span>
          )}
          <label
            className="hidden items-center gap-2 text-[var(--color-text-dim)] sm:flex"
            title="Düğümlerin sıkışıklığı"
          >
            Yoğunluk
            <input
              type="range"
              min={0.4}
              max={2}
              step={0.1}
              value={density}
              onChange={(e) => setDensity(parseFloat(e.target.value))}
              className="w-20 accent-[var(--color-accent)]"
            />
          </label>
          <button
            onClick={() => setDetailOpen(true)}
            className="flex shrink-0 items-center gap-1 rounded-lg border border-[var(--color-border)] px-2 py-1 transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)] lg:hidden"
            aria-haspopup="dialog"
          >
            <MessageSquare size={13} />
            Detay
          </button>
          <button
            onClick={refresh}
            disabled={loading}
            title="Haritayı yenile"
            className="flex shrink-0 items-center gap-1 rounded-lg border border-[var(--color-border)] px-2 py-1 transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)] disabled:opacity-40"
          >
            <RefreshCw size={13} className={loading ? 'animate-spin' : ''} />
            <span className="hidden sm:inline">Yenile</span>
          </button>
        </div>
      </header>

      {graph && visibleGraph && (
        <ExplorerFilters
          filter={filter}
          onChange={setFilter}
          onClear={clearFilter}
          buckets={buckets}
          facets={facets}
          liveCount={liveCount}
          visibleCount={visibleGraph.nodes.length}
          totalCount={graph.nodes.length}
        />
      )}

      <div className="flex min-h-0 flex-1">
        <div
          className="relative min-h-0 min-w-0 flex-1 bg-[var(--color-bg)]"
          role="region"
          aria-label="Workspace haritası"
        >
          <p className="sr-only" aria-live="polite" aria-atomic="true">
            {selectionAnnouncement}
          </p>
          <VisNetworkGraph
            key={workspaceId}
            workspaceId={workspaceId}
            layoutId={`explorer:${workspaceId}`}
            nodes={nodes}
            edges={edges}
            canonicalNodeIds={canonicalNodeIds}
            canonicalReady={ready}
            mode="tree"
            density={density}
            onSelect={handleSelect}
            onNodeDoubleClick={handleDoubleClick}
            focusNodeId={focusKey}
            focusTick={focusTick}
            lite={isMobile}
          />
          {loading && !graph && (
            <div
              role="status"
              className="absolute inset-x-0 top-3 mx-auto w-fit rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-xs text-[var(--color-text-dim)] shadow-lg"
            >
              Harita yükleniyor…
            </div>
          )}
          {!loading && (deepLinkError || error) && (
            <div
              role="alert"
              className="absolute left-1/2 top-3 flex -translate-x-1/2 items-center gap-3 rounded-lg border border-[var(--color-danger)] bg-[var(--color-surface)] px-3 py-2 text-xs shadow-lg"
            >
              <span>{error ? `Harita yüklenemedi: ${error}` : deepLinkError}</span>
              {error ? (
                <button
                  onClick={refresh}
                  className="rounded border border-[var(--color-border)] px-2 py-1 font-medium hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
                >
                  Tekrar dene
                </button>
              ) : (
                <button
                  onClick={fallbackToRoot}
                  className="rounded border border-[var(--color-border)] px-2 py-1 font-medium hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
                >
                  Workspace köküne dön
                </button>
              )}
            </div>
          )}
        </div>

        {/* Side panel: the selected node's full projection — the same DSL an agent
            gets, shown verbatim (reuses the ◱ Özet panel embedded). */}
        <aside className="flex w-[380px] shrink-0 flex-col border-l border-[var(--color-border)] bg-[var(--color-surface)] max-lg:hidden">
          {openSessionButton}
          {/* Keyed by the ref so switching nodes resets the panel's own drill-trail.
              hideHandles: the map is the navigator, the panel only shows the
              projection. fillHeight: the panel owns its scrolling. */}
          <div className="flex min-h-0 flex-1 flex-col">
            <ViewPanel key={selectedKey} target={panelRef} embedded hideHandles fillHeight />
          </div>
        </aside>

        <ExplorerDetailDrawer open={detailOpen} onClose={closeDetail} title="Seçili düğüm detayı">
          {openSessionButton}
          <div className="flex min-h-0 flex-1 flex-col">
            <ViewPanel
              key={`drawer:${selectedKey}`}
              target={panelRef}
              embedded
              hideHandles
              fillHeight
            />
          </div>
        </ExplorerDetailDrawer>
      </div>
    </div>
  )
}
