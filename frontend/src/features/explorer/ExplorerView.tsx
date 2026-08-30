import { useEffect, useState } from 'react'
import {
  ArrowDownToLine,
  ArrowUpToLine,
  Map as MapIcon,
  MessageSquare,
  RefreshCw,
  Search,
  X,
} from 'lucide-react'
import { useRefreshTrigger } from '@/shared/hooks/useRefreshTrigger'
import { refToString, type ViewHandle } from '@/types'
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
  const [overflow, setOverflow] = useState<{
    side: 'parents' | 'children'
    handles: ViewHandle[]
  } | null>(null)
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
          <div className="pointer-events-none absolute inset-x-8 top-4 z-10 grid grid-cols-3 text-[10px] font-bold uppercase tracking-[0.16em] text-[var(--color-text-dim)]">
            <span>Üst bağlantılar</span>
            <span className="text-center text-[var(--color-accent)]">Odak</span>
            <span className="text-right">Alt bağlantılar</span>
          </div>
          <ExplorerGraph
            nodes={nodes}
            edges={edges}
            onNodeClick={select}
            onNodeDoubleClick={focus}
            onOverflowClick={(side, handles) => setOverflow({ side, handles })}
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
          {overflow && (
            <section
              role="dialog"
              aria-modal="false"
              aria-label={`${overflow.side === 'parents' ? 'Üst' : 'Alt'} taşan bağlantılar`}
              className="absolute inset-y-4 right-4 z-20 flex w-[340px] flex-col overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] shadow-xl"
            >
              <header className="flex items-center gap-2 border-b border-[var(--color-border)] px-4 py-3">
                {overflow.side === 'parents' ? (
                  <ArrowUpToLine size={15} className="text-[var(--color-accent)]" />
                ) : (
                  <ArrowDownToLine size={15} className="text-[var(--color-accent)]" />
                )}
                <div>
                  <h2 className="text-sm font-semibold">
                    {overflow.side === 'parents'
                      ? 'Kalan üst bağlantılar'
                      : 'Kalan alt bağlantılar'}
                  </h2>
                  <p className="text-[11px] text-[var(--color-text-dim)]">
                    {overflow.handles.length} ilişki · tek tık seçer, çift tık odaklar
                  </p>
                </div>
                <button
                  onClick={() => setOverflow(null)}
                  aria-label="Bağlantı listesini kapat"
                  className="ml-auto rounded-md p-1 text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
                >
                  <X size={15} />
                </button>
              </header>
              <div className="min-h-0 flex-1 overflow-y-auto p-2">
                {overflow.handles.map((handle) => (
                  <button
                    key={refToString(handle.ref)}
                    onClick={() => select(handle.ref)}
                    onDoubleClick={() => {
                      focus(handle.ref)
                      setOverflow(null)
                    }}
                    className="mb-1 flex w-full items-center gap-3 rounded-lg border border-transparent px-3 py-2 text-left hover:border-[var(--color-border)] hover:bg-[var(--color-surface-2)]"
                  >
                    <span className="min-w-0 flex-1 truncate text-xs font-medium">
                      {handle.label || refToString(handle.ref)}
                    </span>
                    <span className="shrink-0 text-[10px] text-[var(--color-text-dim)]">
                      {handle.ref.kind}
                    </span>
                  </button>
                ))}
              </div>
            </section>
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
