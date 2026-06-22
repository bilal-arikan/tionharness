import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { RefreshCw, Share2, Radio } from 'lucide-react'
import { api } from '../../api'
import type { WorkspaceGraph, WorkspaceNodeType } from '../../types'
import { VisNetworkGraph } from '../graph/VisNetworkGraph'
import { workspaceToVis, EDGE_LEGEND, NODE_LAYERS, type WorkspaceMode } from '../../lib/relationGraph'

interface Props {
  onError: (msg: string) => void
}

// NetworkPanel renders the workspace collaboration network with vis-network.
// Two modes: 'relation' (the full collaboration web) and 'live' (a board-column
// flow where tasks gather under their status column and agents bond to the task
// they're actively working — auto-refreshing on autonomous events). Layer chips
// toggle node types and a density slider tunes packing.
export function NetworkPanel({ onError }: Props) {
  const [graph, setGraph] = useState<WorkspaceGraph | null>(null)
  const [loading, setLoading] = useState(false)
  const [density, setDensity] = useState(1)
  // Default to the live board-column flow so the animated, self-refreshing
  // network is the primary view; users can switch to the static relation web.
  const [mode, setMode] = useState<WorkspaceMode>('live')
  // Visible node layers (agents are always shown). Skills/MCP start hidden to
  // keep the default view focused on the agent/task/flow collaboration core.
  const [visible, setVisible] = useState<Set<WorkspaceNodeType>>(
    () => new Set<WorkspaceNodeType>(['task', 'flow', 'run']),
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

  // Live mode: re-fetch the graph when an autonomous event (task run, schedule)
  // lands, so the flow animates as agents pick up / finish work. A
  // short debounce coalesces bursts. The incremental DataSet update in
  // VisNetworkGraph means the physics engine glides nodes to their new bonds.
  const debounceRef = useRef<number | null>(null)
  useEffect(() => {
    if (mode !== 'live') return
    const unsub = api.subscribeEvents(() => {
      if (debounceRef.current) window.clearTimeout(debounceRef.current)
      debounceRef.current = window.setTimeout(() => load(), 600)
    })
    return () => {
      unsub()
      if (debounceRef.current) window.clearTimeout(debounceRef.current)
    }
  }, [mode, load])

  const { nodes, edges } = useMemo(
    () => (graph ? workspaceToVis(graph, visible, mode) : { nodes: [], edges: [] }),
    [graph, visible, mode],
  )

  // In live mode tasks/columns are intrinsic; the flow/skill/MCP layers stay
  // user-toggleable (an agent's flows, skills and MCP servers drift with it).
  const layers =
    mode === 'live'
      ? NODE_LAYERS.filter((l) => l.type === 'flow' || l.type === 'skill' || l.type === 'mcp' || l.type === 'run')
      : NODE_LAYERS.filter((l) => l.type !== 'run') // 'run' is a live-only archive layer

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
          {mode === 'relation' &&
            EDGE_LEGEND.map((l) => (
              <span key={l.kind} className="flex items-center gap-1 text-[var(--color-text-dim)]">
                <span className="inline-block h-0.5 w-4 rounded" style={{ background: l.color }} />
                {l.label}
              </span>
            ))}
          {mode === 'live' && (
            <span className="flex items-center gap-1 text-[var(--color-accent)]">
              <Radio size={12} className="animate-pulse" /> canlı — olaylarda kendiliğinden güncellenir
            </span>
          )}
          {/* Mode toggle: relationship web vs live board-column flow. */}
          <div className="flex gap-0.5 rounded-md bg-[var(--color-surface-2)] p-0.5">
            <button
              onClick={() => setMode('relation')}
              className={`flex items-center gap-1 rounded px-2 py-0.5 transition ${
                mode === 'relation' ? 'bg-[var(--color-accent)] text-white' : 'text-[var(--color-text-dim)]'
              }`}
              title="İlişki ağı (tüm bağlar)"
            >
              <Share2 size={13} /> İlişki
            </button>
            <button
              onClick={() => setMode('live')}
              className={`flex items-center gap-1 rounded px-2 py-0.5 transition ${
                mode === 'live' ? 'bg-[var(--color-accent)] text-white' : 'text-[var(--color-text-dim)]'
              }`}
              title="Canlı sütun akışı (görevler durum sütunlarında, ajan aktif göreve bağlanır)"
            >
              <Radio size={13} /> Canlı
            </button>
          </div>
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
        <span className="text-[var(--color-text-dim)]">
          {mode === 'live' ? 'Sütun akışı · katmanlar:' : 'Katmanlar:'}
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
          <VisNetworkGraph nodes={nodes} edges={edges} mode={mode} density={density} />
        )}
      </div>
    </div>
  )
}
