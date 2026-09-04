import { useCallback, useEffect, useMemo, useState } from 'react'
import { Map as MapIcon, MessageSquare, RefreshCw } from 'lucide-react'
import { useRefreshTrigger } from '@/shared/hooks/useRefreshTrigger'
import { useIsMobile } from '@/shared/hooks/useMediaQuery'
import { refToString } from '@/types'
import { ViewPanel } from '@/features/view/ViewPanel'
import { VisNetworkGraph } from '@/features/network/VisNetworkGraph'
import { ExplorerDetailDrawer } from './ExplorerDetailDrawer'
import { ExplorerSearch, type ExplorerSearchResult } from './ExplorerSearch'
import { displayLabel, KIND_LABEL, ROOT_KEY } from './explorerVis'
import { useExplorerGraph } from './useExplorerGraph'

interface Props {
  workspaceId: string
  onError: (msg: string) => void
  // Open a session transcript (wired by App to setView('chat') + selectSession):
  // a double click on a session node jumps to its conversation.
  onOpenSession?: (sessionId: string) => void
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
  focusNode,
  onFocusNode,
}: Props) {
  const [search, setSearch] = useState('')
  const [density, setDensity] = useState(1)
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
    selectedRef,
    selectedKey,
    focusKey,
    focusTick,
    selectKey,
    fallbackToRoot,
    refresh,
  } = useExplorerGraph({ onError, search, initialFocus: focusNode, onFocus: onFocusNode })

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
    if (!needle || !graph) return []
    return graph.nodes.flatMap((handle) => {
      const key = refToString(handle.ref)
      const label = key === ROOT_KEY ? 'Workspace' : displayLabel(handle)
      if (!label.toLowerCase().includes(needle) && !key.toLowerCase().includes(needle)) return []
      return [{ key, label, kind: handle.ref.kind, selected: key === selectedKey }]
    })
  }, [graph, search, selectedKey])

  const selectedHandle = graph?.nodes.find((handle) => refToString(handle.ref) === selectedKey)
  const selectionAnnouncement = selectedHandle
    ? `Seçili düğüm ${displayLabel(selectedHandle)} (${KIND_LABEL[selectedHandle.ref.kind]}).`
    : ''

  const openSessionButton = selectedRef.kind === 'session' && onOpenSession && (
    <button
      onClick={() => onOpenSession(selectedRef.id)}
      className="flex items-center gap-1.5 border-b border-[var(--color-border)] px-3 py-2 text-xs text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
    >
      <MessageSquare size={13} />
      Sohbeti aç · {selectedRef.id}
    </button>
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
            mode="relation"
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
            <ViewPanel key={selectedKey} target={selectedRef} embedded hideHandles fillHeight />
          </div>
        </aside>

        <ExplorerDetailDrawer open={detailOpen} onClose={closeDetail} title="Seçili düğüm detayı">
          {openSessionButton}
          <div className="flex min-h-0 flex-1 flex-col">
            <ViewPanel
              key={`drawer:${selectedKey}`}
              target={selectedRef}
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
