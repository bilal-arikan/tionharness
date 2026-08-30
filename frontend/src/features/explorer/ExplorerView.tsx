import { useEffect, useState } from 'react'
import { Map as MapIcon, MessageSquare, RefreshCw, Search, X } from 'lucide-react'
import { useRefreshTrigger } from '@/shared/hooks/useRefreshTrigger'
import { refToString } from '@/types'
import { ViewPanel } from '@/features/view/ViewPanel'
import { ExplorerGraph } from './ExplorerGraph'
import { useExplorerGraph } from './useExplorerGraph'

interface Props {
  onError: (msg: string) => void
  // Open a session transcript (wired by App to setView('chat') + selectSession),
  // so a session node on the map is one click from its conversation.
  onOpenSession?: (sessionId: string) => void
  // Deep-link: the selected node's ref string, restored from the URL on entry and
  // reported back on every selection change so the map is shareable/restorable.
  focusNode?: string | null
  onFocusNode?: (refString: string) => void
}

// ExplorerView is the "Harita" screen: a one-hop focus graph over the View layer.
// A click selects for the side panel; a double click moves focus.
//
// This is deliberately SEPARATE from the Network screen: the network is a
// relationship graph of running agent instances, this is a state drill-down.
export function ExplorerView({ onError, onOpenSession, focusNode, onFocusNode }: Props) {
  const [search, setSearch] = useState('')
  const { nodes, edges, select, focus, selectedRef, focusLoading, focusError, refreshFocused } =
    useExplorerGraph({ search, onError, initialFocus: focusNode, onFocus: onFocusNode })

  // Live update refreshes only the current focus neighborhood.
  const tick = useRefreshTrigger('explorer')
  useEffect(() => {
    refreshFocused()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tick])

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <header className="flex items-center gap-3 border-b border-[var(--color-border)] py-3 max-md:px-3 md:px-6">
        <MapIcon size={16} className="shrink-0 text-[var(--color-accent)]" />
        <span className="shrink-0 text-sm font-semibold">Harita</span>

        {/* In-map search: dims non-matching visible nodes (focus+context). */}
        <div className="relative ml-2 hidden items-center sm:flex">
          <Search
            size={13}
            className="pointer-events-none absolute left-2 text-[var(--color-text-dim)]"
          />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="haritada ara…"
            className="w-44 rounded-md border border-[var(--color-border)] bg-transparent py-1 pl-7 pr-6 text-xs text-[var(--color-text)] focus:border-[var(--color-accent)] focus:outline-none"
          />
          {search && (
            <button
              onClick={() => setSearch('')}
              title="Aramayı temizle"
              className="absolute right-1.5 text-[var(--color-text-dim)] hover:text-[var(--color-danger)]"
            >
              <X size={13} />
            </button>
          )}
        </div>

        <div className="ml-auto flex items-center gap-2 text-xs">
          <button
            onClick={refreshFocused}
            title="Odak çevresini yenile"
            className="flex shrink-0 items-center gap-1 rounded-lg border border-[var(--color-border)] px-2 py-1 transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
          >
            <RefreshCw size={13} />
            <span className="hidden sm:inline">Yenile</span>
          </button>
        </div>
      </header>

      <div className="flex min-h-0 flex-1">
        <div className="relative min-h-0 flex-1 bg-[var(--color-bg)]">
          <ExplorerGraph
            nodes={nodes}
            edges={edges}
            onNodeClick={select}
            onNodeDoubleClick={focus}
          />
          {focusLoading && (
            <div
              role="status"
              className="absolute inset-x-0 top-3 mx-auto w-fit rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-xs text-[var(--color-text-dim)] shadow-lg"
            >
              Odak çevresi yükleniyor…
            </div>
          )}
          {!focusLoading && focusError && (
            <div
              role="alert"
              className="absolute left-1/2 top-3 flex -translate-x-1/2 items-center gap-3 rounded-lg border border-[var(--color-danger)] bg-[var(--color-surface)] px-3 py-2 text-xs shadow-lg"
            >
              <span>Odak çevresi yüklenemedi: {focusError}</span>
              <button
                onClick={refreshFocused}
                className="rounded border border-[var(--color-border)] px-2 py-1 font-medium hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
              >
                Tekrar dene
              </button>
            </div>
          )}
        </div>

        {/* Side panel: the selected node's full projection — the same DSL an agent
            gets, shown verbatim (reuses the ◱ Özet panel embedded). */}
        <aside className="flex w-[380px] shrink-0 flex-col border-l border-[var(--color-border)] bg-[var(--color-surface)] max-lg:hidden">
          {selectedRef.kind === 'session' && onOpenSession && (
            <button
              onClick={() => onOpenSession(selectedRef.id)}
              className="flex items-center gap-1.5 border-b border-[var(--color-border)] px-3 py-2 text-xs text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
            >
              <MessageSquare size={13} />
              Sohbeti aç · {selectedRef.id}
            </button>
          )}
          {/* Keyed by the ref so switching nodes resets the panel's own drill-trail. */}
          {/* The panel owns its own scrolling in fillHeight mode, so the host must
              NOT scroll too — otherwise the pinned footer scrolls away with it. */}
          <div className="flex min-h-0 flex-1 flex-col">
            {/* hideHandles: the map is the navigator here, so the panel shows only
                the projection an agent would get — no drill chips beside it. */}
            <ViewPanel
              key={refToString(selectedRef)}
              target={selectedRef}
              embedded
              hideHandles
              fillHeight
            />
          </div>
        </aside>
      </div>
    </div>
  )
}
