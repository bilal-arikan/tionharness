import { useEffect, useRef, useState } from 'react'
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
  // Deep-link: the focus node's ref string, restored from the URL on entry and
  // reported back on every focus change so the map is shareable/restorable.
  focusNode?: string | null
  onFocusNode?: (refString: string | null) => void
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
  const [detailOpen, setDetailOpen] = useState(false)
  const overflowCloseRef = useRef<HTMLButtonElement>(null)
  const detailCloseRef = useRef<HTMLButtonElement>(null)
  const overflowDialogRef = useRef<HTMLElement>(null)
  const detailDialogRef = useRef<HTMLElement>(null)
  const overflowTriggerRef = useRef<HTMLElement | null>(null)
  const detailTriggerRef = useRef<HTMLElement | null>(null)
  const detailOpenTimerRef = useRef<number | null>(null)
  const {
    nodes,
    edges,
    select,
    focus,
    selectedRef,
    focusLoading,
    focusError,
    deepLinkError,
    fallbackToRoot,
    refreshFocused,
  } = useExplorerGraph({ search, onError, initialFocus: focusNode, onFocus: onFocusNode })
  const searchResults = search.trim()
    ? nodes.filter(
        (node) =>
          !node.data.overflow &&
          (node.data.label.toLowerCase().includes(search.trim().toLowerCase()) ||
            refToString(node.data.ref).toLowerCase().includes(search.trim().toLowerCase())),
      )
    : []
  const focusedNode = nodes.find((node) => node.data.focus)
  const focusAnnouncement = focusedNode
    ? `Harita odağı ${focusedNode.data.label}. ${focusedNode.data.childCount ?? 0} alt bağlantı.`
    : ''

  const isNarrowScreen = () =>
    typeof window.matchMedia === 'function' && window.matchMedia('(max-width: 1023px)').matches

  const selectNode = (ref: Parameters<typeof select>[0]) => {
    select(ref)
    if (isNarrowScreen()) {
      detailTriggerRef.current = document.activeElement as HTMLElement | null
      if (detailOpenTimerRef.current !== null) window.clearTimeout(detailOpenTimerRef.current)
      // Preserve the single-click select / double-click focus contract: defer the
      // drawer until the browser's double-click window has had time to complete.
      detailOpenTimerRef.current = window.setTimeout(() => {
        setDetailOpen(true)
        detailOpenTimerRef.current = null
      }, 250)
    }
  }

  const applyFocus = (ref: Parameters<typeof focus>[0]) => {
    if (detailOpenTimerRef.current !== null) {
      window.clearTimeout(detailOpenTimerRef.current)
      detailOpenTimerRef.current = null
    }
    setDetailOpen(false)
    focus(ref)
  }

  const closeOverflow = () => {
    setOverflow(null)
    window.setTimeout(() => overflowTriggerRef.current?.focus())
  }

  const closeDetail = () => {
    setDetailOpen(false)
    window.setTimeout(() => detailTriggerRef.current?.focus())
  }

  // Live update refreshes only the current focus neighborhood.
  const tick = useRefreshTrigger('explorer')
  useEffect(() => {
    refreshFocused()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tick])

  useEffect(() => {
    if (overflow) overflowCloseRef.current?.focus()
  }, [overflow])

  useEffect(() => {
    if (detailOpen) detailCloseRef.current?.focus()
  }, [detailOpen])

  useEffect(
    () => () => {
      if (detailOpenTimerRef.current !== null) window.clearTimeout(detailOpenTimerRef.current)
    },
    [],
  )

  useEffect(() => {
    if (!overflow && !detailOpen) return
    const escape = (event: globalThis.KeyboardEvent) => {
      if (event.key === 'Escape') {
        if (overflow) closeOverflow()
        else closeDetail()
        return
      }
      if (event.key !== 'Tab') return
      const dialog = overflow ? overflowDialogRef.current : detailDialogRef.current
      const focusable = dialog?.querySelectorAll<HTMLElement>(
        'button:not([disabled]), a[href], input:not([disabled]), [tabindex]:not([tabindex="-1"])',
      )
      if (!focusable?.length) return
      const first = focusable[0]
      const last = focusable[focusable.length - 1]
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault()
        last.focus()
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault()
        first.focus()
      }
    }
    document.addEventListener('keydown', escape)
    return () => document.removeEventListener('keydown', escape)
  })

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
          {search && searchResults.length > 0 && (
            <div
              role="listbox"
              aria-label="Harita arama sonuçları"
              className="absolute left-0 top-8 z-30 w-72 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-1 shadow-xl"
            >
              {searchResults.map((node) => (
                <button
                  key={node.id}
                  role="option"
                  aria-selected={node.data.selected}
                  onClick={() => selectNode(node.data.ref)}
                  onDoubleClick={() => applyFocus(node.data.ref)}
                  className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-xs hover:bg-[var(--color-surface-2)]"
                >
                  <span className="min-w-0 flex-1 truncate">{node.data.label}</span>
                  <span className="shrink-0 text-[10px] text-[var(--color-text-dim)]">
                    {node.data.ref.kind}
                  </span>
                </button>
              ))}
            </div>
          )}
        </div>

        <div className="ml-auto flex items-center gap-2 text-xs">
          <button
            onClick={(event) => {
              detailTriggerRef.current = event.currentTarget
              setDetailOpen(true)
            }}
            className="flex shrink-0 items-center gap-1 rounded-lg border border-[var(--color-border)] px-2 py-1 transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)] lg:hidden"
            aria-haspopup="dialog"
          >
            <MessageSquare size={13} />
            Detay
          </button>
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
        <div className="relative min-h-0 min-w-0 flex-1 bg-[var(--color-bg)]">
          <p className="sr-only" aria-live="polite" aria-atomic="true">
            {focusAnnouncement}
          </p>
          <p id="explorer-keyboard-help" className="sr-only">
            Ok tuşları katmanlar arasında gezinir. Enter veya Boşluk seçer. Shift+Enter odağı
            değiştirir.
          </p>
          <div className="pointer-events-none absolute inset-x-8 top-4 z-10 grid grid-cols-3 text-[10px] font-bold uppercase tracking-[0.16em] text-[var(--color-text-dim)]">
            <span>Üst bağlantılar</span>
            <span className="text-center text-[var(--color-accent)]">Odak</span>
            <span className="text-right">Alt bağlantılar</span>
          </div>
          <ExplorerGraph
            nodes={nodes}
            edges={edges}
            onNodeClick={selectNode}
            onNodeDoubleClick={applyFocus}
            onOverflowClick={(side, handles) => {
              overflowTriggerRef.current = document.activeElement as HTMLElement | null
              setOverflow({ side, handles })
            }}
          />
          {focusLoading && (
            <div
              role="status"
              className="absolute inset-x-0 top-3 mx-auto w-fit rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-xs text-[var(--color-text-dim)] shadow-lg"
            >
              Odak çevresi yükleniyor…
            </div>
          )}
          {!focusLoading && (deepLinkError || focusError) && (
            <div
              role="alert"
              className="absolute left-1/2 top-3 flex -translate-x-1/2 items-center gap-3 rounded-lg border border-[var(--color-danger)] bg-[var(--color-surface)] px-3 py-2 text-xs shadow-lg"
            >
              <span>Odak çevresi yüklenemedi: {deepLinkError || focusError}</span>
              <button
                onClick={fallbackToRoot}
                className="rounded border border-[var(--color-border)] px-2 py-1 font-medium hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
              >
                Workspace köküne dön
              </button>
              {focusError && !deepLinkError && (
                <button
                  onClick={refreshFocused}
                  className="rounded border border-[var(--color-border)] px-2 py-1 font-medium hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
                >
                  Tekrar dene
                </button>
              )}
            </div>
          )}
          {overflow && (
            <section
              ref={overflowDialogRef}
              role="dialog"
              aria-modal="true"
              aria-label={`${overflow.side === 'parents' ? 'Üst' : 'Alt'} taşan bağlantılar`}
              aria-describedby="overflow-help"
              className="absolute inset-y-4 right-4 z-20 flex w-[340px] flex-col overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] shadow-xl max-md:fixed max-md:inset-x-2 max-md:bottom-2 max-md:top-auto max-md:max-h-[75vh] max-md:w-auto"
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
                  <p id="overflow-help" className="text-[11px] text-[var(--color-text-dim)]">
                    {overflow.handles.length} ilişki · tek tık seçer, çift tık odaklar
                  </p>
                </div>
                <button
                  ref={overflowCloseRef}
                  onClick={closeOverflow}
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
                      applyFocus(handle.ref)
                      closeOverflow()
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

        {detailOpen && (
          <div className="fixed inset-0 z-40 bg-black/40 lg:hidden" onMouseDown={closeDetail}>
            <section
              ref={detailDialogRef}
              role="dialog"
              aria-modal="true"
              aria-label="Seçili düğüm detayı"
              className="absolute inset-x-2 bottom-2 top-[10vh] flex flex-col overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] shadow-xl"
              onMouseDown={(event) => event.stopPropagation()}
            >
              <header className="flex items-center gap-2 border-b border-[var(--color-border)] px-3 py-2">
                <h2 className="min-w-0 flex-1 truncate text-sm font-semibold">
                  Seçili düğüm detayı
                </h2>
                <button
                  ref={detailCloseRef}
                  onClick={closeDetail}
                  aria-label="Düğüm detayını kapat"
                  className="rounded-md p-1 text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
                >
                  <X size={16} />
                </button>
              </header>
              {selectedRef.kind === 'session' && onOpenSession && (
                <button
                  onClick={() => onOpenSession(selectedRef.id)}
                  className="flex items-center gap-1.5 border-b border-[var(--color-border)] px-3 py-2 text-xs text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
                >
                  <MessageSquare size={13} />
                  Sohbeti aç · {selectedRef.id}
                </button>
              )}
              <div className="flex min-h-0 flex-1 flex-col">
                <ViewPanel
                  key={`drawer:${refToString(selectedRef)}`}
                  target={selectedRef}
                  embedded
                  hideHandles
                  fillHeight
                />
              </div>
            </section>
          </div>
        )}
      </div>
    </div>
  )
}
