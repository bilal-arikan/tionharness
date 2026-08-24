import { useCallback, useEffect, useMemo, useState } from 'react'
import { RefreshCw } from 'lucide-react'
import { api } from '@/api'
import { useRefreshTrigger } from '@/shared/hooks/useRefreshTrigger'
import type { WorkspaceGraph, WorkspaceNodeType, BoardColumnDef } from '@/types'
import { VisNetworkGraph } from './VisNetworkGraph'
import { workspaceToVis, NODE_LAYERS, type WorkspaceMode } from './relationGraph'
import { NetworkFilters } from './NetworkFilters'
import { emptyNetworkFilter, filterGraph, type NetworkFilter } from './networkFilter'
import { useIsMobile } from '@/shared/hooks/useMediaQuery'

interface Props {
  onError: (msg: string) => void
  // Open a session transcript — wired by App to setView('chat') + selectSession.
  // Clicking an agent instance (or a run-history card) navigates to the session
  // that instance is driving, so "which agent is on which session" is one click.
  onOpenSession?: (sessionId: string) => void
}

// sessionIdFromNodeId extracts the backing session id from a clickable node id.
// Agent instances carry it after '#' (agent:<agentID>#<sessionID>); run-history
// cards are run:<sessionID>. Other node types (task/flow/skill/mcp/anchors) have
// no session, so they return null and the click is ignored.
function sessionIdFromNodeId(id: string): string | null {
  if (id.startsWith('agent:')) {
    const hash = id.indexOf('#')
    return hash >= 0 ? id.slice(hash + 1) : null
  }
  if (id.startsWith('run:')) return id.slice('run:'.length)
  return null
}

// NetworkPanel renders the workspace collaboration network with vis-network.
// Two modes: 'relation' (the full collaboration web) and 'live' (a board-column
// flow where tasks gather under their status column and agents bond to the task
// they're actively working — auto-refreshing on autonomous events). Layer chips
// toggle node types and a density slider tunes packing.
//
// Agents on this canvas are RUNTIME INSTANCES: the backend emits one agent node
// per in-flight session (chat / task / flow / schedule / spawn / worker), so a
// busy agent appears once per run and an idle agent does not appear at all.
export function NetworkPanel({ onError, onOpenSession }: Props) {
  const [graph, setGraph] = useState<WorkspaceGraph | null>(null)
  const [loading, setLoading] = useState(false)
  // User-defined Kanban columns (mirrors the Board column editor). When set,
  // the live-mode column anchors in the network are taken from here instead of
  // the built-in five-status defaults.
  const [boardColumns, setBoardColumns] = useState<BoardColumnDef[]>([])
  const [density, setDensity] = useState(1)
  // The network is always the live board-column flow now — the static relation
  // web was dropped from the UI, so there is no mode toggle. `mode` stays a
  // constant (relationGraph still branches on it) in case relation is reinstated.
  const mode: WorkspaceMode = 'live'
  // Visible node layers (agents are always shown). Skills/MCP start hidden to
  // keep the default view focused on the agent/task/flow collaboration core.
  const [visible, setVisible] = useState<Set<WorkspaceNodeType>>(
    () => new Set<WorkspaceNodeType>(['task', 'flow', 'run']),
  )
  // Board-style facet filter (search / agent / kind / status / tag / archive).
  // Applied to the raw graph before it is mapped to vis nodes.
  const [filter, setFilter] = useState<NetworkFilter>(() => emptyNetworkFilter())

  // Phones get the lightweight vis-network render (no shadows/curved edges) so
  // pan/zoom stays smooth on low-power GPUs.
  const isMobile = useIsMobile()

  // Clicking an agent instance / run card opens its session transcript.
  const handleSelect = useCallback(
    (id: string | null) => {
      if (!id || !onOpenSession) return
      const sid = sessionIdFromNodeId(id)
      if (sid) onOpenSession(sid)
    },
    [onOpenSession],
  )

  const toggleLayer = (t: WorkspaceNodeType) =>
    setVisible((prev) => {
      const next = new Set(prev)
      if (next.has(t)) next.delete(t)
      else next.add(t)
      return next
    })

  const load = useCallback(() => {
    setLoading(true)
    Promise.all([api.workspaceGraph(), api.getWorkspaceSettings()])
      .then(([g, s]: [WorkspaceGraph, { boardColumns?: BoardColumnDef[] }]) => {
        setGraph(g)
        if (s && Array.isArray(s.boardColumns)) setBoardColumns(s.boardColumns)
      })
      .catch((e) => onError((e as Error).message))
      .finally(() => setLoading(false))
  }, [onError])

  useEffect(() => {
    load()
  }, [load])

  // Board editor save hook: re-pull BOTH the workspace graph and settings
  // whenever the Board column editor saves. load() is the canonical refresh
  // (Promise.all of graph + settings); calling it covers the column-shape
  // change AND any concurrent task mutation that may have happened alongside.
  // Works without SSE and across workspaces.
  useEffect(() => {
    const handler = () => {
      load()
    }
    window.addEventListener('tionharness:board-columns-changed', handler)
    return () => window.removeEventListener('tionharness:board-columns-changed', handler)
  }, [load])

  // Cross-window live sync: App.tsx's central SSE handler bumps the 'network'
  // refresh signal on every task / agent / flow / session / schedule / spawn
  // event in the active workspace. The 200ms debounce in the dispatcher
  // coalesces bursts so a flurry of CRUDs triggers a single re-fetch.
  //
  // This replaces the per-panel live-mode SSE subscription AND extends to
  // relation mode (the previous design was live-only because the user opted
  // out of SSE there). With the central dispatcher the workspace filter is
  // applied once in App.tsx, so we can just trust the bump and re-pull.
  const networkTick = useRefreshTrigger('network')
  useEffect(() => {
    load()
  }, [networkTick, load])

  // Apply the facet filter to the raw graph first; the vis mapping then runs on
  // the narrowed set (edges to dropped nodes and orphaned skill/MCP icons fall
  // out inside filterGraph).
  const filteredGraph = useMemo(() => (graph ? filterGraph(graph, filter) : null), [graph, filter])

  const { nodes, edges } = useMemo(
    () =>
      filteredGraph
        ? workspaceToVis(filteredGraph, visible, mode, boardColumns)
        : { nodes: [], edges: [] },
    [filteredGraph, visible, mode, boardColumns],
  )

  // In live mode tasks/columns are intrinsic; the flow/skill/MCP layers stay
  // user-toggleable (an agent's flows, skills and MCP servers drift with it).
  const layers =
    mode === 'live'
      ? NODE_LAYERS.filter(
          (l) => l.type === 'flow' || l.type === 'skill' || l.type === 'mcp' || l.type === 'run',
        )
      : NODE_LAYERS.filter((l) => l.type !== 'run') // 'run' is a live-only archive layer

  const isEmpty = graph && graph.nodes.length === 0
  // Graph has content but the active filter matched nothing — distinct from the
  // truly-empty workspace so we can hint that clearing the filter helps.
  const filteredEmpty = !isEmpty && graph && filteredGraph && filteredGraph.nodes.length === 0

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      {/* Title bar: screen name + right-aligned graph stats + far-right refresh. */}
      <header className="flex items-center gap-3 border-b border-[var(--color-border)] py-3 max-md:px-3 md:px-6">
        <span className="shrink-0 text-sm font-semibold">Ağ</span>
        {graph && (
          <span className="ml-auto truncate text-xs text-[var(--color-text-dim)]">
            {graph.stats.agents} aktif ajan
            {graph.stats.agentsTotal ? ` / ${graph.stats.agentsTotal}` : ''} · {graph.stats.tasks}{' '}
            görev · {graph.stats.flows} akış · {graph.stats.skills ?? 0} beceri ·{' '}
            {graph.stats.mcp ?? 0} MCP
          </span>
        )}
        <button
          onClick={load}
          disabled={loading}
          className={`flex shrink-0 items-center gap-1 rounded-lg border border-[var(--color-border)] px-2 py-1 text-xs transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)] disabled:opacity-40 ${graph ? '' : 'ml-auto'}`}
          title="Yenile"
        >
          <RefreshCw size={13} className={loading ? 'animate-spin' : ''} />
          <span className="hidden sm:inline">Yenile</span>
        </button>
      </header>

      {/* Toolbar: one merged row — board-style facet filters (search /
          agent / kind / status / tag / archive) PLUS the node-type layer chips
          and the density slider, passed in as children. */}
      {graph && filteredGraph && (
        <NetworkFilters
          filter={filter}
          onChange={setFilter}
          onClear={() => setFilter(emptyNetworkFilter())}
          graph={graph}
          visibleCount={filteredGraph.nodes.length}
          boardColumns={boardColumns}
        >
          <span className="text-[var(--color-text-dim)]">
            {mode === 'live' ? 'katmanlar:' : 'Katmanlar:'}
          </span>
          {layers.map((l) => {
            const on = visible.has(l.type)
            return (
              <button
                key={l.type}
                onClick={() => toggleLayer(l.type)}
                className={`flex items-center gap-1 rounded-full border px-2 py-0.5 transition ${
                  on
                    ? 'border-transparent text-white'
                    : 'border-[var(--color-border)] text-[var(--color-text-dim)] opacity-60'
                }`}
                style={on ? { background: l.color } : undefined}
              >
                <span
                  className="inline-block h-2 w-2 rounded-full"
                  style={{ background: on ? '#fff' : l.color }}
                />
                {l.label}
              </button>
            )
          })}
          <label
            className="flex items-center gap-2 text-[var(--color-text-dim)]"
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
              className="w-24 accent-[var(--color-accent)]"
            />
            <span className="w-7 tabular-nums">{density.toFixed(1)}×</span>
          </label>
        </NetworkFilters>
      )}

      <div className="relative min-h-0 flex-1 bg-[var(--color-bg)]">
        {isEmpty ? (
          <div className="flex h-full items-center justify-center px-6 text-center text-sm text-[var(--color-text-dim)]">
            Henüz görselleştirilecek bir şey yok. Görev veya akış ekledikçe ağ burada belirir;
            ajanlar yalnızca çalışırken (sohbet, görev, akış, otomasyon, spawn) görünür.
          </div>
        ) : filteredEmpty ? (
          <div className="flex h-full flex-col items-center justify-center gap-2 px-6 text-center text-sm text-[var(--color-text-dim)]">
            Filtreye uyan düğüm yok.
            <button
              onClick={() => setFilter(emptyNetworkFilter())}
              className="rounded border border-[var(--color-accent)] bg-[var(--color-accent-soft)] px-2 py-1 text-xs text-[var(--color-accent)]"
            >
              Filtreleri temizle
            </button>
          </div>
        ) : (
          <VisNetworkGraph
            nodes={nodes}
            edges={edges}
            mode={mode}
            density={density}
            onSelect={handleSelect}
            lite={isMobile}
          />
        )}
      </div>
    </div>
  )
}
