import { useCallback, useEffect, useState } from 'react'
import { useNodesState, useEdgesState, type Edge } from '@xyflow/react'
import { Loader2 } from 'lucide-react'
import { api } from '../../api'
import type { FlowNodeEvent } from '../../api/flows'
import { Markdown } from '../markdown/Markdown'
import { FlowCanvas } from '../flow/FlowCanvas'
import { NodeInspector } from '../flow/NodeInspector'
import {
  graphToReactFlow,
  reactFlowToGraph,
  blankNode,
  nextNodeId,
  type FlowRFNode,
} from '../../lib/flowGraph'
import type { Agent, Flow, FlowNode, FlowNodeType, FlowRun, FlowState } from '../../types'

interface Props {
  agents: Agent[]
  onError: (msg: string) => void
}

const NODE_TYPES: { value: FlowNodeType; label: string; icon: string }[] = [
  { value: 'agent', label: 'Ajan', icon: '🤖' },
  { value: 'branch', label: 'Dallanma', icon: '🔀' },
  { value: 'parallel', label: 'Paralel', icon: '⚡' },
]

// FlowsPanel is the visual protocol builder: pick a flow, edit it on a drag-and-
// drop node canvas (React Flow), save, run with an input, and watch per-node
// progress stream live on the canvas and in the trace below.
export function FlowsPanel({ agents, onError }: Props) {
  const [flows, setFlows] = useState<Flow[]>([])
  const [selectedId, setSelectedId] = useState<string | null>(null)

  // Editor state for the selected flow.
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [start, setStart] = useState('')
  const [nodes, setNodes, onNodesChange] = useNodesState<FlowRFNode>([])
  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>([])
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null)

  // Run state.
  const [input, setInput] = useState('')
  const [running, setRunning] = useState(false)
  const [run, setRun] = useState<FlowRun | null>(null)
  const [liveNodes, setLiveNodes] = useState<FlowNodeEvent[]>([])

  const loadFlows = useCallback(() => {
    api.listFlows().then(setFlows).catch((e) => onError(e.message))
  }, [onError])

  useEffect(() => loadFlows(), [loadFlows])

  const selectFlow = useCallback(
    (f: Flow) => {
      setSelectedId(f.id)
      setName(f.name)
      setDescription(f.description)
      setRun(null)
      setInput('')
      setLiveNodes([])
      setSelectedNodeId(null)
      try {
        const g = f.graph ? JSON.parse(f.graph) : { start: '', nodes: [] }
        const { nodes: rn, edges: re } = graphToReactFlow({
          start: g.start ?? '',
          nodes: Array.isArray(g.nodes) ? g.nodes : [],
        })
        setNodes(rn)
        setEdges(re)
        setStart(g.start ?? '')
      } catch {
        setNodes([])
        setEdges([])
        setStart('')
      }
    },
    [setNodes, setEdges],
  )

  const createFlow = async () => {
    const n = prompt('Akış adı:')
    if (!n) return
    try {
      const f = await api.createFlow(n)
      setFlows((prev) => [f, ...prev])
      selectFlow(f)
    } catch (e) {
      onError((e as Error).message)
    }
  }

  // addNode appends a blank node of the given type near the canvas origin and
  // makes it the start node if none is set yet.
  const addNode = (type: FlowNodeType) => {
    const existing = nodes.map((n) => n.data.node)
    const id = nextNodeId(existing)
    const node = blankNode(id, type, agents[0]?.id ?? '')
    const offset = nodes.length * 30
    const rf: FlowRFNode = {
      id,
      type,
      position: { x: 80 + offset, y: 80 + offset },
      data: { node, isStart: !start },
    }
    setNodes((prev) => [...prev, rf])
    if (!start) setStart(id)
  }

  // patchSelected updates the selected node's intrinsic fields. A type change or
  // a shrunk branch list prunes now-invalid outgoing edges so the graph stays
  // consistent on save.
  const patchSelected = (patch: Partial<FlowNode>) => {
    if (!selectedNodeId) return
    let prune = false
    setNodes((prev) =>
      prev.map((rn) => {
        if (rn.id !== selectedNodeId) return rn
        const before = rn.data.node
        const merged = { ...before, ...patch }
        if (patch.type && patch.type !== before.type) prune = true
        if (
          patch.branches &&
          patch.branches.length < (before.branches?.length ?? 0)
        )
          prune = true
        return { ...rn, type: merged.type, data: { ...rn.data, node: merged } }
      }),
    )
    if (prune) {
      setEdges((eds) => eds.filter((e) => e.source !== selectedNodeId))
    }
  }

  const makeStart = () => {
    if (!selectedNodeId) return
    setStart(selectedNodeId)
    setNodes((prev) =>
      prev.map((rn) => ({ ...rn, data: { ...rn.data, isStart: rn.id === selectedNodeId } })),
    )
  }

  const deleteSelected = () => {
    if (!selectedNodeId) return
    setNodes((prev) => prev.filter((rn) => rn.id !== selectedNodeId))
    setEdges((eds) => eds.filter((e) => e.source !== selectedNodeId && e.target !== selectedNodeId))
    if (start === selectedNodeId) setStart('')
    setSelectedNodeId(null)
  }

  const saveFlow = async () => {
    if (!selectedId) return
    try {
      const graph = reactFlowToGraph(nodes, edges, start)
      const f = await api.updateFlow(selectedId, name, description, graph)
      setFlows((prev) => prev.map((x) => (x.id === f.id ? f : x)))
      onError('') // clear
    } catch (e) {
      onError((e as Error).message)
    }
  }

  const removeFlow = async (f: Flow) => {
    if (!confirm(`"${f.name}" akışı silinsin mi?`)) return
    try {
      await api.deleteFlow(f.id)
      if (selectedId === f.id) setSelectedId(null)
      loadFlows()
    } catch (e) {
      onError((e as Error).message)
    }
  }

  // setNodeStatus paints a node's live run state (running glow / done ring).
  const setNodeStatus = (nodeId: string, status: 'running' | 'done' | undefined) => {
    setNodes((prev) =>
      prev.map((rn) => (rn.id === nodeId ? { ...rn, data: { ...rn.data, status } } : rn)),
    )
  }

  const doRun = async () => {
    if (!selectedId) return
    setRunning(true)
    setRun(null)
    setLiveNodes([])
    setNodes((prev) => prev.map((rn) => ({ ...rn, data: { ...rn.data, status: undefined } })))
    try {
      await saveFlow() // persist edits before running
      await api.runFlowStreamStandalone(selectedId, input, {
        onNode: (ev) => {
          setNodeStatus(ev.nodeId, ev.phase === 'start' ? 'running' : 'done')
          setLiveNodes((prev) => {
            if (ev.phase === 'start') return [...prev, ev]
            const i = prev.findIndex((n) => n.nodeId === ev.nodeId && n.output === undefined)
            if (i < 0) return [...prev, ev]
            const next = [...prev]
            next[i] = ev
            return next
          })
        },
        onReply: (r) => setRun(r.run),
        onError: (e) => onError(e),
      })
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setRunning(false)
    }
  }

  const selectedNode = nodes.find((n) => n.id === selectedNodeId)?.data.node ?? null
  const trace: FlowState | null = run?.state ? safeParse(run.state) : null

  return (
    <div className="flex min-h-0 flex-1">
      {/* Flow list */}
      <div className="w-56 flex-shrink-0 overflow-y-auto border-r border-[var(--color-border)] p-3">
        <button
          onClick={createFlow}
          className="mb-3 w-full rounded-lg bg-[var(--color-accent)] px-3 py-2 text-sm font-medium text-white hover:opacity-90"
        >
          + Yeni akış
        </button>
        <ul className="space-y-1">
          {flows.map((f) => (
            <li key={f.id}>
              <button
                onClick={() => selectFlow(f)}
                className={`flex w-full items-start justify-between rounded-lg px-3 py-2 text-left text-sm ${
                  selectedId === f.id
                    ? 'bg-[var(--color-surface-2)]'
                    : 'hover:bg-[var(--color-surface-2)]'
                }`}
              >
                <span className="min-w-0 flex-1">
                  <span className="block truncate">{f.name}</span>
                  {f.description && (
                    <span className="mt-0.5 block truncate text-xs text-[var(--color-text-dim)]">
                      {f.description}
                    </span>
                  )}
                </span>
                <span
                  onClick={(e) => {
                    e.stopPropagation()
                    removeFlow(f)
                  }}
                  className="ml-2 text-xs text-[var(--color-text-dim)] hover:text-[var(--color-danger)]"
                >
                  ✕
                </span>
              </button>
            </li>
          ))}
          {flows.length === 0 && (
            <li className="text-sm text-[var(--color-text-dim)]">Henüz akış yok.</li>
          )}
        </ul>
      </div>

      {/* Editor + run */}
      {!selectedId ? (
        <div className="flex-1 p-6">
          <p className="text-sm text-[var(--color-text-dim)]">
            Soldan bir akış seçin veya yeni bir akış oluşturun.
          </p>
        </div>
      ) : (
        <div className="flex min-w-0 flex-1 flex-col">
          {/* Meta toolbar: name + description side by side */}
          <div className="flex items-center gap-2 border-b border-[var(--color-border)] p-3">
            <input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Akış adı"
              className="w-56 flex-shrink-0 rounded bg-[var(--color-surface-2)] px-3 py-2 text-sm font-medium outline-none"
            />
            <input
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="Açıklama — bu akış ne yapar? (isteğe bağlı)"
              className="min-w-0 flex-1 rounded bg-[var(--color-surface-2)] px-3 py-2 text-sm text-[var(--color-text-dim)] outline-none"
            />
            <button
              onClick={saveFlow}
              className="flex-shrink-0 rounded-lg bg-[var(--color-accent)] px-3 py-2 text-sm font-medium text-white hover:opacity-90"
            >
              Kaydet
            </button>
          </div>

          {/* Node palette + canvas + inspector */}
          <div className="flex min-h-0 flex-1">
            <div className="w-32 flex-shrink-0 space-y-2 overflow-y-auto border-r border-[var(--color-border)] p-2">
              <div className="px-1 text-xs font-semibold text-[var(--color-text-dim)]">
                Node ekle
              </div>
              {NODE_TYPES.map((t) => (
                <button
                  key={t.value}
                  onClick={() => addNode(t.value)}
                  className="flex w-full items-center gap-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-2 py-2 text-left text-xs hover:border-[var(--color-accent)]"
                >
                  <span className="text-base">{t.icon}</span>
                  <span>{t.label}</span>
                </button>
              ))}
            </div>
            <div className="min-w-0 flex-1">
              <FlowCanvas
                agents={agents}
                nodes={nodes}
                edges={edges}
                onNodesChange={onNodesChange}
                onEdgesChange={onEdgesChange}
                setEdges={setEdges}
                onSelect={setSelectedNodeId}
              />
            </div>
            <div className="w-72 flex-shrink-0 overflow-y-auto border-l border-[var(--color-border)] p-3">
              {selectedNode ? (
                <NodeInspector
                  node={selectedNode}
                  agents={agents}
                  isStart={start === selectedNode.id}
                  onPatch={patchSelected}
                  onMakeStart={makeStart}
                  onDelete={deleteSelected}
                />
              ) : (
                <p className="text-xs text-[var(--color-text-dim)]">
                  Düzenlemek için bir node seçin. Bağlantı için bir node'un tutamağından
                  diğerine sürükleyin.
                </p>
              )}
            </div>
          </div>

          {/* Run */}
          <div className="max-h-[40%] overflow-y-auto border-t border-[var(--color-border)] p-4">
            <h3 className="mb-2 text-sm font-semibold">Çalıştır</h3>
            <div className="flex gap-2">
              <textarea
                value={input}
                onChange={(e) => setInput(e.target.value)}
                placeholder="Girdi (akışa {{input}} olarak geçer)"
                rows={1}
                className="flex-1 rounded bg-[var(--color-surface-2)] px-3 py-2 text-sm outline-none"
              />
              <button
                onClick={doRun}
                disabled={running}
                className="rounded-lg bg-[var(--color-accent)] px-4 py-2 text-sm font-medium text-white hover:opacity-90 disabled:opacity-50"
              >
                {running ? 'Çalışıyor…' : '▶ Çalıştır'}
              </button>
            </div>

            {/* Live node progress while running (before the final run lands). */}
            {!run && liveNodes.length > 0 && (
              <div className="mt-4 border-t border-[var(--color-border)] pt-3">
                <div className="mb-2 text-xs text-[var(--color-text-dim)]">Canlı ilerleme</div>
                <ol className="space-y-2">
                  {liveNodes.map((n, i) => (
                    <li key={`${n.nodeId}-${i}`} className="rounded bg-[var(--color-surface-2)] p-2 text-sm">
                      <div className="mb-1 flex items-center gap-1.5 text-xs text-[var(--color-text-dim)]">
                        {n.output === undefined && (
                          <Loader2 size={12} className="animate-spin text-[var(--color-accent)]" />
                        )}
                        {i + 1}. [{n.type}] {n.title}
                      </div>
                      {n.output === undefined ? (
                        <span className="text-xs italic text-[var(--color-text-dim)]">çalışıyor…</span>
                      ) : n.type === 'branch' ? (
                        <div className="whitespace-pre-wrap">{n.output}</div>
                      ) : (
                        <Markdown>{n.output}</Markdown>
                      )}
                    </li>
                  ))}
                </ol>
              </div>
            )}

            {run && (
              <div className="mt-4 border-t border-[var(--color-border)] pt-3">
                <div className="mb-2 text-xs">
                  Durum:{' '}
                  <span className={run.status === 'success' ? 'text-green-400' : 'text-red-400'}>
                    {run.status}
                  </span>
                  {run.error && <span className="ml-2 text-red-400">· {run.error}</span>}
                </div>
                <ol className="space-y-2">
                  {(trace?.trace ?? []).map((t, i) => (
                    <li key={i} className="rounded bg-[var(--color-surface-2)] p-2 text-sm">
                      <div className="mb-1 text-xs text-[var(--color-text-dim)]">
                        {i + 1}. [{t.type}] {t.title}
                      </div>
                      {t.type === 'branch' ? (
                        <div className="whitespace-pre-wrap">{t.output}</div>
                      ) : (
                        <Markdown>{t.output}</Markdown>
                      )}
                    </li>
                  ))}
                </ol>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

function safeParse(s: string): FlowState | null {
  try {
    return JSON.parse(s) as FlowState
  } catch {
    return null
  }
}
