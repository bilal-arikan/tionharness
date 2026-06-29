import { useCallback, useEffect, useMemo, useState } from 'react'
import { X, Anchor, Palette } from 'lucide-react'
import { api } from '../../api'
import type { MemoryGraph, MemoryGraphNode } from '../../types'
import { VisNetworkGraph } from './VisNetworkGraph'
import { memoryToVis, MEMORY_KIND_COLOR, MEMORY_KIND_LABEL, fmtDate } from '../../lib/relationGraph'

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
// linked, and the physics engine clusters them. A threshold slider tunes which
// links survive; a density slider tunes packing; hovering a node highlights its
// neighbourhood; clicking a node opens a detail panel with the full content.
export function MemoryGraphView({ agentId, onError }: Props) {
  const [graph, setGraph] = useState<MemoryGraph | null>(null)
  const [threshold, setThreshold] = useState(0.18)
  const [density, setDensity] = useState(1)
  const [loading, setLoading] = useState(false)
  const [selectedId, setSelectedId] = useState<string | null>(null)
  // Layout toggles: per-kind draggable anchors (cluster by kind) and
  // connected-component coloring (cluster by topic).
  const [kindAnchors, setKindAnchors] = useState(true)
  const [clusterColor, setClusterColor] = useState(true)

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
    () => (graph ? memoryToVis(graph, { kindAnchors, clusterColor }) : { nodes: [], edges: [] }),
    [graph, kindAnchors, clusterColor],
  )

  const selected: MemoryGraphNode | null = useMemo(
    () => (selectedId ? graph?.nodes.find((n) => n.id === selectedId) ?? null : null),
    [selectedId, graph],
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
        {/* Layout toggles: kind anchors (cluster by kind) + cluster coloring. */}
        <button
          onClick={() => setKindAnchors((v) => !v)}
          title="Türe göre sürüklenebilir çapalar — hafızalar türlerine göre kümelenir"
          className={`flex items-center gap-1 rounded-full border px-2 py-0.5 transition ${
            kindAnchors
              ? 'border-transparent bg-[var(--color-accent)] text-white'
              : 'border-[var(--color-border)] text-[var(--color-text-dim)]'
          }`}
        >
          <Anchor size={12} /> Tür çapaları
        </button>
        <button
          onClick={() => setClusterColor((v) => !v)}
          title="Bağlı bileşenleri (konu grupları) farklı renklerle boya"
          className={`flex items-center gap-1 rounded-full border px-2 py-0.5 transition ${
            clusterColor
              ? 'border-transparent bg-[var(--color-accent)] text-white'
              : 'border-[var(--color-border)] text-[var(--color-text-dim)]'
          }`}
        >
          <Palette size={12} /> Küme rengi
        </button>
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
            className="w-28 accent-[var(--color-accent)]"
          />
          <span className="w-7 tabular-nums">{density.toFixed(1)}×</span>
        </label>
      </div>

      <div className="relative min-h-0 flex-1 bg-[var(--color-bg)]">
        {isEmpty ? (
          <div className="flex h-full items-center justify-center px-6 text-center text-sm text-[var(--color-text-dim)]">
            Bu ajanın henüz hafızası yok. Belge ekle veya yansıma üret.
          </div>
        ) : (
          <>
            <VisNetworkGraph
              nodes={nodes}
              edges={edges}
              density={density}
              highlightNeighbors
              onSelect={setSelectedId}
            />
            {selected && (
              <div className="absolute right-3 top-3 z-10 w-72 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-3 shadow-xl">
                <div className="mb-2 flex items-center justify-between gap-2">
                  <span
                    className="flex items-center gap-1.5 rounded-full px-2 py-0.5 text-[11px] font-medium text-white"
                    style={{ background: MEMORY_KIND_COLOR[selected.kind] ?? '#64748b' }}
                  >
                    {MEMORY_KIND_LABEL[selected.kind] ?? selected.kind}
                  </span>
                  <button
                    onClick={() => setSelectedId(null)}
                    className="rounded p-0.5 text-[var(--color-text-dim)] transition hover:text-[var(--color-text)]"
                    title="Kapat"
                  >
                    <X size={14} />
                  </button>
                </div>
                <p className="max-h-60 overflow-y-auto whitespace-pre-wrap text-xs leading-relaxed text-[var(--color-text)]">
                  {selected.content}
                </p>
                <div className="mt-2 flex items-center justify-between border-t border-[var(--color-border)] pt-2 text-[11px] text-[var(--color-text-dim)]">
                  <span>{selected.degree} bağ</span>
                  {selected.createdAt > 0 && <span>{fmtDate(selected.createdAt)}</span>}
                </div>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  )
}
