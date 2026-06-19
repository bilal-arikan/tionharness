import { useCallback, useEffect, useMemo, useState } from 'react'
import { api } from '../../api'
import type { MemoryGraph } from '../../types'
import { VisNetworkGraph } from './VisNetworkGraph'
import { memoryToVis, MEMORY_KIND_COLOR } from '../../lib/relationGraph'

interface Props {
  agentId: string
  onError: (msg: string) => void
}

const KIND_LEGEND: { kind: string; label: string }[] = [
  { kind: 'document', label: 'Belge' },
  { kind: 'journal', label: 'Günlük' },
  { kind: 'reflection', label: 'Yansıma' },
]

// MemoryGraphView renders an agent's memory knowledge graph with vis-network:
// every memory is a node, similar memories (lexical cosine ≥ threshold) are
// linked, and the physics engine clusters them. A slider tunes the threshold.
export function MemoryGraphView({ agentId, onError }: Props) {
  const [graph, setGraph] = useState<MemoryGraph | null>(null)
  const [threshold, setThreshold] = useState(0.18)
  const [loading, setLoading] = useState(false)

  const load = useCallback(
    (t: number) => {
      setLoading(true)
      api
        .memoryGraph(agentId, t)
        .then(setGraph)
        .catch((e) => onError((e as Error).message))
        .finally(() => setLoading(false))
    },
    [agentId, onError],
  )

  useEffect(() => {
    load(threshold)
  }, [load, threshold])

  const { nodes, edges } = useMemo(
    () => (graph ? memoryToVis(graph) : { nodes: [], edges: [] }),
    [graph],
  )

  const linked = graph?.nodes.filter((n) => n.degree > 0).length ?? 0
  const isEmpty = graph && graph.nodes.length === 0

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex flex-wrap items-center gap-3 border-b border-[var(--color-border)] px-1 py-2 text-xs">
        {graph && (
          <span className="text-[var(--color-text-dim)]">
            {graph.nodes.length} hafıza · {graph.edges.length} bağ · {linked} bağlı
          </span>
        )}
        {KIND_LEGEND.map((k) => (
          <span key={k.kind} className="flex items-center gap-1 text-[var(--color-text-dim)]">
            <span
              className="inline-block h-2 w-2 rounded-full"
              style={{ background: MEMORY_KIND_COLOR[k.kind] }}
            />
            {k.label}
          </span>
        ))}
        <label className="ml-auto flex items-center gap-2 text-[var(--color-text-dim)]">
          Benzerlik eşiği: {threshold.toFixed(2)}
          <input
            type="range"
            min={0.05}
            max={0.6}
            step={0.01}
            value={threshold}
            onChange={(e) => setThreshold(parseFloat(e.target.value))}
            className="w-32 accent-[var(--color-accent)]"
            disabled={loading}
          />
        </label>
      </div>

      <div className="relative min-h-0 flex-1 bg-[var(--color-bg)]">
        {isEmpty ? (
          <div className="flex h-full items-center justify-center px-6 text-center text-sm text-[var(--color-text-dim)]">
            Bu ajanın henüz hafızası yok. Belge ekle veya yansıma üret.
          </div>
        ) : (
          <VisNetworkGraph nodes={nodes} edges={edges} />
        )}
      </div>
    </div>
  )
}
