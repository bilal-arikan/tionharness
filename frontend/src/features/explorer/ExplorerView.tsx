import { useEffect, useState } from 'react'
import { Map as MapIcon, MessageSquare, RefreshCw, Search, X } from 'lucide-react'
import { useRefreshTrigger } from '@/shared/hooks/useRefreshTrigger'
import { VIEW_LENS_LABEL, refToString } from '@/types'
import type { ViewLens } from '@/types'
import { ViewPanel } from '@/features/view/ViewPanel'
import { ExplorerGraph } from './ExplorerGraph'
import { useExplorerGraph } from './useExplorerGraph'

const LENSES: ViewLens[] = ['health', 'stale', 'recent', 'errors']

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

// ExplorerView is the "Harita" screen: a semantic-zoom drill-down over the View
// layer. Click the root, then a category, then a member, expanding one layer at a
// time; the selected node's full projection shows in the side panel — the exact
// bytes an agent would receive (verifiability comes free).
//
// This is deliberately SEPARATE from the Network screen: the network is a
// relationship graph of running agent instances, this is a state drill-down.
export function ExplorerView({ onError, onOpenSession, focusNode, onFocusNode }: Props) {
  const [lens, setLens] = useState<ViewLens>('health')
  const [search, setSearch] = useState('')
  const { nodes, edges, toggle, selectedRef, refreshExpanded } = useExplorerGraph({
    lens,
    search,
    onError,
    initialSelected: focusNode,
    onSelect: onFocusNode,
  })

  // Live update: App's central SSE handler bumps the 'explorer' signal on the same
  // lifecycle events as the network. Only OPEN branches are refreshed (not the whole
  // map), so a busy workspace stays cheap.
  const tick = useRefreshTrigger('explorer')
  useEffect(() => {
    refreshExpanded()
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
          <label className="flex items-center gap-1 text-[var(--color-text-dim)]">
            mercek
            <select
              value={lens}
              onChange={(e) => setLens(e.target.value as ViewLens)}
              className="rounded-md border border-[var(--color-border)] bg-transparent px-2 py-1 text-[var(--color-text)]"
            >
              {LENSES.map((l) => (
                <option key={l} value={l}>
                  {VIEW_LENS_LABEL[l]}
                </option>
              ))}
            </select>
          </label>
          <button
            onClick={refreshExpanded}
            title="Açık dalları yenile"
            className="flex shrink-0 items-center gap-1 rounded-lg border border-[var(--color-border)] px-2 py-1 transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
          >
            <RefreshCw size={13} />
            <span className="hidden sm:inline">Yenile</span>
          </button>
        </div>
      </header>

      <div className="flex min-h-0 flex-1">
        <div className="relative min-h-0 flex-1 bg-[var(--color-bg)]">
          <ExplorerGraph nodes={nodes} edges={edges} onNodeClick={toggle} />
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
          <div className="min-h-0 flex-1 overflow-auto">
            <ViewPanel key={refToString(selectedRef)} target={selectedRef} embedded />
          </div>
        </aside>
      </div>
    </div>
  )
}
