import { useCallback, useEffect, useMemo, useState } from 'react'
import { RefreshCw } from 'lucide-react'
import { api } from '../../api'
import type { WorkspaceGraph, WorkspaceNodeType } from '../../types'
import { VisNetworkGraph } from '../graph/VisNetworkGraph'
import { workspaceToVis, EDGE_LEGEND, NODE_LAYERS } from '../../lib/relationGraph'

interface Props {
  onError: (msg: string) => void
}

// NetworkPanel renders the workspace collaboration network with vis-network:
// agents, tasks, flows, skills and MCP servers as nodes, their relationships as
// color-coded edges. A real physics engine (Fizik) or a hierarchical tree (Ağaç)
// lays them out; layer chips toggle node types and a density slider tunes packing.
export function NetworkPanel({ onError }: Props) {
  const [graph, setGraph] = useState<WorkspaceGraph | null>(null)
  const [loading, setLoading] = useState(false)
  const [density, setDensity] = useState(1)
  // Visible node layers (agents are always shown). Skills/MCP start hidden to
  // keep the default view focused on the agent/task/flow collaboration core.
  const [visible, setVisible] = useState<Set<WorkspaceNodeType>>(
    () => new Set<WorkspaceNodeType>(['task', 'flow']),
  )

  const toggleLayer = (t: WorkspaceNodeType) =>
    setVisible((prev) => {
      const next = new Set(prev)
      next.has(t) ? next.delete(t) : next.add(t)
      return next
    })

  const load = useCallback(() => {
    setLoading(true)
    api
      .workspaceGraph()
      .then(setGraph)
      .catch((e) => onError((e as Error).message))
      .finally(() => setLoading(false))
  }, [onError])

  useEffect(() => {
    load()
  }, [load])

  const { nodes, edges } = useMemo(
    () => (graph ? workspaceToVis(graph, visible) : { nodes: [], edges: [] }),
    [graph, visible],
  )

  const isEmpty = graph && graph.nodes.length === 0

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      {/* Toolbar row 1: stats + layout toggle + refresh */}
      <div className="flex flex-wrap items-center gap-3 border-b border-[var(--color-border)] px-4 py-2 text-xs">
        {graph && (
          <span className="text-[var(--color-text-dim)]">
            {graph.stats.agents} ajan · {graph.stats.tasks} görev · {graph.stats.flows} akış ·{' '}
            {graph.stats.skills ?? 0} beceri · {graph.stats.mcp ?? 0} MCP
          </span>
        )}
        <div className="ml-auto flex items-center gap-3">
          {EDGE_LEGEND.map((l) => (
            <span key={l.kind} className="flex items-center gap-1 text-[var(--color-text-dim)]">
              <span className="inline-block h-0.5 w-4 rounded" style={{ background: l.color }} />
              {l.label}
            </span>
          ))}
          <button
            onClick={load}
            disabled={loading}
            className="flex items-center gap-1 rounded-lg border border-[var(--color-border)] px-2 py-1 transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)] disabled:opacity-40"
            title="Yenile"
          >
            <RefreshCw size={13} className={loading ? 'animate-spin' : ''} />
            Yenile
          </button>
        </div>
      </div>

      {/* Toolbar row 2: layer chips + density slider */}
      <div className="flex flex-wrap items-center gap-3 border-b border-[var(--color-border)] px-4 py-1.5 text-xs">
        <span className="text-[var(--color-text-dim)]">Katmanlar:</span>
        {NODE_LAYERS.map((l) => {
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
        <label className="ml-auto flex items-center gap-2 text-[var(--color-text-dim)]" title="Düğümlerin sıkışıklığı">
          Yoğunluk
          <input
            type="range"
            min={0.4}
            max={2}
            step={0.1}
            value={density}
            onChange={(e) => setDensity(parseFloat(e.target.value))}
            className="w-32 accent-[var(--color-accent)]"
          />
          <span className="w-7 tabular-nums">{density.toFixed(1)}×</span>
        </label>
      </div>

      <div className="relative min-h-0 flex-1 bg-[var(--color-bg)]">
        {isEmpty ? (
          <div className="flex h-full items-center justify-center px-6 text-center text-sm text-[var(--color-text-dim)]">
            Henüz görselleştirilecek bir ilişki yok. Ajan, görev veya akış ekledikçe ağ burada
            belirir.
          </div>
        ) : (
          <VisNetworkGraph nodes={nodes} edges={edges} density={density} />
        )}
      </div>
    </div>
  )
}
