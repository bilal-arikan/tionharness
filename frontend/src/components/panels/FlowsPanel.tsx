import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNodesState, useEdgesState, type Edge } from '@xyflow/react'
import { Loader2, XCircle, FolderOpen } from 'lucide-react'
import { api } from '../../api'
import { CopyPathButton } from '../CopyPathButton'
import { useRegisterDirty } from '../../lib/dirtySignals'
import type { FlowNodeEvent } from '../../api/flows'
import { Markdown } from '../markdown/Markdown'
import { FlowCanvas, type EdgeStyle } from '../flow/FlowCanvas'
import { TemplatePreview } from '../flow/TemplatePreview'
import { RunView } from '../flow/RunView'
import { NodeInspector } from '../flow/NodeInspector'
import { FLOW_TEMPLATES, type FlowTemplate } from '../../lib/flowTemplates'
import {
  graphToReactFlow,
  reactFlowToGraph,
  autoLayout,
  blankNode,
  nextNodeId,
  type FlowRFNode,
} from '../../lib/flowGraph'
import type { Agent, Flow, FlowNode, FlowNodeType, FlowRun, FlowState } from '../../types'
import { Button } from '../common'

interface Props {
  agents: Agent[]
  onError: (msg: string) => void
  // Deep-link: when set, open this flow's run history (from the Activity screen).
  openFlowId?: string | null
}

const NODE_TYPES: { value: FlowNodeType; label: string; icon: string }[] = [
  { value: 'agent', label: 'Ajan', icon: '🤖' },
  { value: 'branch', label: 'Dallanma', icon: '🔀' },
  { value: 'parallel', label: 'Paralel', icon: '⚡' },
  { value: 'delay', label: 'Bekle', icon: '⏱️' },
  { value: 'transform', label: 'Birleştir', icon: '🧩' },
]

const EDGE_STYLES: { value: EdgeStyle; label: string }[] = [
  { value: 'default', label: 'Eğri' },
  { value: 'smoothstep', label: 'Yumuşak' },
  { value: 'step', label: 'Basamak' },
  { value: 'straight', label: 'Düz' },
]

// FlowsPanel is the visual protocol builder: pick a flow, edit it on a drag-and-
// drop node canvas (React Flow), save, run with an input, and watch per-node
// progress stream live on the canvas and in the trace below.
export function FlowsPanel({ agents, onError, openFlowId }: Props) {
  const [flows, setFlows] = useState<Flow[]>([])
  const [selectedId, setSelectedId] = useState<string | null>(null)
  // Absolute path of the selected flow's on-disk JSON file (for copy / reveal).
  const [flowPath, setFlowPath] = useState('')
  // Left-column tab: own flows, read-only template gallery, or run history.
  const [tab, setTab] = useState<'flows' | 'templates' | 'runs'>('flows')
  const [templateId, setTemplateId] = useState<string | null>(null)
  // Run history (all flows, newest first) + the selected run for the read-only viewer.
  const [runs, setRuns] = useState<FlowRun[]>([])
  const [selectedRunId, setSelectedRunId] = useState<string | null>(null)
  // Left-list search (adapts to the active tab: flow/template name, or a run's
  // flow name).
  const [q, setQ] = useState('')

  // Editor state for the selected flow.
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [start, setStart] = useState('')
  const [nodes, setNodes, onNodesChange] = useNodesState<FlowRFNode>([])
  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>([])
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null)
  // Edge presentation (cosmetic). Stored per-flow in the graph; localStorage
  // holds the last-used edge style as the default for flows that have none.
  const [edgeStyle, setEdgeStyle] = useState<EdgeStyle>(
    () => (localStorage.getItem('swarmgo.flowEdgeStyle') as EdgeStyle) || 'default',
  )
  const [animated, setAnimated] = useState(false)
  const changeEdgeStyle = (s: EdgeStyle) => {
    setEdgeStyle(s)
    localStorage.setItem('swarmgo.flowEdgeStyle', s)
  }

  // Run state.
  const [input, setInput] = useState('')
  const [running, setRunning] = useState(false)
  const [run, setRun] = useState<FlowRun | null>(null)
  const [liveNodes, setLiveNodes] = useState<FlowNodeEvent[]>([])

  const loadFlows = useCallback(() => {
    api.listFlows().then(setFlows).catch((e) => onError(e.message))
  }, [onError])

  useEffect(() => loadFlows(), [loadFlows])

  // While the Koşular tab is open, load all flow runs and poll every 3s so
  // in-progress runs advance live. Polling stops when leaving the tab.
  useEffect(() => {
    if (tab !== 'runs') return
    let alive = true
    const tick = () => {
      api
        .listAllFlowRuns()
        .then((rs) => alive && setRuns(rs))
        .catch(() => {})
    }
    tick()
    const id = setInterval(tick, 3000)
    return () => {
      alive = false
      clearInterval(id)
    }
  }, [tab])

  // Deep-link from the Activity screen: open this flow's run history and select
  // its latest run (runs are newest-first). Consumed once per target so polling
  // doesn't keep re-selecting.
  const consumedFlowTarget = useRef<string | null>(null)
  useEffect(() => {
    if (openFlowId) setTab('runs')
  }, [openFlowId])
  useEffect(() => {
    if (!openFlowId || tab !== 'runs' || consumedFlowTarget.current === openFlowId) return
    const latest = runs.find((r) => r.flowId === openFlowId)
    if (latest) {
      setSelectedRunId(latest.id)
      consumedFlowTarget.current = openFlowId
    }
  }, [openFlowId, tab, runs])

  const selectFlow = useCallback(
    (f: Flow) => {
      setSelectedId(f.id)
      setFlowPath('')
      api.flowPath(f.id).then((r) => setFlowPath(r.path)).catch(() => setFlowPath(''))
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
        setEdgeStyle(
          (g.edgeStyle as EdgeStyle) ||
            ((localStorage.getItem('swarmgo.flowEdgeStyle') as EdgeStyle) || 'default'),
        )
        setAnimated(!!g.animated)
      } catch {
        setNodes([])
        setEdges([])
        setStart('')
        setAnimated(false)
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

  // instantiateTemplate creates a new editable flow from a template's graph,
  // then switches to it for editing. Template agent nodes are unassigned; the
  // backend requires every agent node to have an agentId, so we seed each with
  // the first available agent as a placeholder for the user to reassign.
  const instantiateTemplate = async (t: FlowTemplate) => {
    const defaultAgent = agents[0]?.id
    if (!defaultAgent) {
      onError('Şablondan akış oluşturmak için önce en az bir ajan oluşturun.')
      return
    }
    const graph = {
      ...t.graph,
      nodes: t.graph.nodes.map((n) =>
        n.type === 'agent' && !n.agentId ? { ...n, agentId: defaultAgent } : n,
      ),
    }
    try {
      const f = await api.createFlow(t.name, t.description, graph)
      setFlows((prev) => [f, ...prev])
      setTab('flows')
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

  // Node actions are id-based so both the inspector (on the selected node) and
  // each node's NodeToolbar can invoke them.
  const makeStartNode = (id: string) => {
    setStart(id)
    setNodes((prev) => prev.map((rn) => ({ ...rn, data: { ...rn.data, isStart: rn.id === id } })))
  }

  const deleteNode = (id: string) => {
    setNodes((prev) => prev.filter((rn) => rn.id !== id))
    setEdges((eds) => eds.filter((e) => e.source !== id && e.target !== id))
    if (start === id) setStart('')
    if (selectedNodeId === id) setSelectedNodeId(null)
  }

  // duplicateNode clones a node (new id, offset position, not start) without its
  // connections — the copy starts unwired.
  const duplicateNode = (id: string) => {
    setNodes((prev) => {
      const src = prev.find((rn) => rn.id === id)
      if (!src) return prev
      const newId = nextNodeId(prev.map((rn) => rn.data.node))
      const copy: FlowRFNode = {
        id: newId,
        type: src.type,
        position: { x: src.position.x + 40, y: src.position.y + 40 },
        data: { node: { ...src.data.node, id: newId }, isStart: false },
      }
      return [...prev, copy]
    })
  }

  const makeStart = () => {
    if (selectedNodeId) makeStartNode(selectedNodeId)
  }
  const deleteSelected = () => {
    if (selectedNodeId) deleteNode(selectedNodeId)
  }

  // autoArrange re-lays-out the graph with the layered grid algorithm and
  // applies the computed positions to the canvas nodes.
  const autoArrange = () => {
    const graph = reactFlowToGraph(nodes, edges, start)
    const pos = autoLayout(graph)
    setNodes((prev) =>
      prev.map((rn) => (pos[rn.id] ? { ...rn, position: { x: pos[rn.id].x, y: pos[rn.id].y } } : rn)),
    )
  }

  const saveFlow = async () => {
    if (!selectedId) return
    try {
      const graph = reactFlowToGraph(nodes, edges, start)
      graph.edgeStyle = edgeStyle
      graph.animated = animated
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

  // setNodeStatus paints a node's live run state (running glow / done ring /
  // error ring).
  const setNodeStatus = (nodeId: string, status: 'running' | 'done' | 'error' | undefined) => {
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
          const status = ev.phase === 'start' ? 'running' : ev.phase === 'error' ? 'error' : 'done'
          setNodeStatus(ev.nodeId, status)
          setLiveNodes((prev) => {
            if (ev.phase === 'start') return [...prev, ev]
            // done/error: replace the pending entry for this node (still running).
            const i = prev.findIndex((n) => n.nodeId === ev.nodeId && n.output === undefined && n.error === undefined)
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

  // Unsaved-edits (dirty) signal for the nav "Akışlar" item + workspace label:
  // compare the live editor (name/description + structural graph) to the stored
  // flow. Cosmetic-only fields (edgeStyle/animated) are ignored so they don't
  // raise a false amber dot. Best-effort — a parse failure reads as "not dirty".
  const flowDirty = useMemo(() => {
    const stored = flows.find((f) => f.id === selectedId)
    if (!selectedId || !stored) return false
    if (name !== stored.name || description !== (stored.description ?? '')) return true
    try {
      const norm = (g: { start?: string; nodes?: unknown }) =>
        JSON.stringify({ start: g.start ?? '', nodes: g.nodes ?? [] })
      const cur = reactFlowToGraph(nodes, edges, start)
      const prev = stored.graph ? JSON.parse(stored.graph) : {}
      return norm(cur) !== norm(prev)
    } catch {
      return false
    }
  }, [flows, selectedId, name, description, nodes, edges, start])
  useRegisterDirty('flows', flowDirty)

  const selectedNode = nodes.find((n) => n.id === selectedNodeId)?.data.node ?? null
  const trace: FlowState | null = run?.state ? safeParse(run.state) : null
  const selectedTemplate = FLOW_TEMPLATES.find((t) => t.id === templateId) ?? null
  // Derived from the polled `runs` list, so the selected run refreshes live.
  const selectedRun = runs.find((r) => r.id === selectedRunId) ?? null

  return (
    <div className="flex min-h-0 flex-1">
      {/* Flow list / template gallery */}
      <div className="w-56 flex-shrink-0 overflow-y-auto border-r border-[var(--color-border)] p-3">
        {/* Tab switch */}
        <div className="mb-3 flex gap-1 rounded-lg bg-[var(--color-surface-2)] p-1 text-xs">
          {(['flows', 'templates', 'runs'] as const).map((t) => (
            <button
              key={t}
              onClick={() => setTab(t)}
              className={`flex-1 rounded-md px-1.5 py-1 ${
                tab === t ? 'bg-[var(--color-accent)] text-white' : 'text-[var(--color-text-dim)]'
              }`}
            >
              {t === 'flows' ? 'Akışlarım' : t === 'templates' ? 'Şablonlar' : 'Koşular'}
            </button>
          ))}
        </div>

        {/* Search (filters the active tab's list). */}
        <input
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder={tab === 'runs' ? 'Koşu ara (akış adı)…' : tab === 'templates' ? 'Şablon ara…' : 'Akış ara…'}
          className="mb-2 w-full rounded-md border border-[var(--color-border)] bg-[var(--color-bg)] px-2.5 py-1.5 text-xs outline-none focus:border-[var(--color-accent)]"
        />

        {tab === 'templates' ? (
          <ul className="space-y-1">
            {FLOW_TEMPLATES.filter((t) => t.name.toLowerCase().includes(q.trim().toLowerCase())).map((t) => (
              <li key={t.id}>
                <button
                  onClick={() => setTemplateId(t.id)}
                  className={`w-full rounded-lg px-3 py-2 text-left text-sm ${
                    templateId === t.id
                      ? 'bg-[var(--color-surface-2)]'
                      : 'hover:bg-[var(--color-surface-2)]'
                  }`}
                >
                  <span className="block truncate">{t.name}</span>
                  <span className="mt-0.5 block truncate text-xs text-[var(--color-text-dim)]">
                    {t.description}
                  </span>
                </button>
              </li>
            ))}
          </ul>
        ) : tab === 'runs' ? (
          <ul className="space-y-1">
            {runs
              .filter((rn) => {
                const qq = q.trim().toLowerCase()
                if (!qq) return true
                return (flows.find((f) => f.id === rn.flowId)?.name ?? '').toLowerCase().includes(qq)
              })
              .map((rn) => {
              const fname = flows.find((f) => f.id === rn.flowId)?.name ?? '（silinmiş akış）'
              const badge =
                rn.status === 'success' ? '✓' : rn.status === 'failure' ? '✕' : '▶'
              const badgeColor =
                rn.status === 'success'
                  ? 'text-[var(--color-success)]'
                  : rn.status === 'failure'
                    ? 'text-[var(--color-danger)]'
                    : 'text-[var(--color-accent)]'
              return (
                <li key={rn.id}>
                  <button
                    onClick={() => setSelectedRunId(rn.id)}
                    className={`flex w-full items-start gap-2 rounded-lg px-3 py-2 text-left text-sm ${
                      selectedRunId === rn.id
                        ? 'bg-[var(--color-surface-2)]'
                        : 'hover:bg-[var(--color-surface-2)]'
                    }`}
                  >
                    <span className={`mt-0.5 text-xs ${badgeColor}`}>{badge}</span>
                    <span className="min-w-0 flex-1">
                      <span className="block truncate">{fname}</span>
                      <span className="mt-0.5 block truncate text-xs text-[var(--color-text-dim)]">
                        {new Date(rn.createdAt * 1000).toLocaleString()}
                      </span>
                    </span>
                  </button>
                </li>
              )
            })}
            {runs.length === 0 && (
              <li className="text-sm text-[var(--color-text-dim)]">Henüz koşu yok.</li>
            )}
          </ul>
        ) : (
          <>
        <Button onClick={createFlow} size="lg" className="mb-3 w-full">
          + Yeni akış
        </Button>
        <ul className="space-y-1">
          {flows.filter((f) => f.name.toLowerCase().includes(q.trim().toLowerCase())).map((f) => (
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
          </>
        )}
      </div>

      {/* Main: template preview or flow editor */}
      {tab === 'templates' ? (
        <div className="flex min-w-0 flex-1 flex-col">
          {!selectedTemplate ? (
            <div className="flex-1 p-6">
              <p className="text-sm text-[var(--color-text-dim)]">
                Soldan bir şablon seçin — yapısını önizleyin, sonra "Bu şablondan akış oluştur" deyin.
              </p>
            </div>
          ) : (
            <>
              <div className="flex items-center gap-2 border-b border-[var(--color-border)] p-3">
                <div className="min-w-0 flex-1">
                  <div className="truncate text-sm font-medium">{selectedTemplate.name}</div>
                  <div className="truncate text-xs text-[var(--color-text-dim)]">
                    {selectedTemplate.description}
                  </div>
                </div>
                <span className="flex-shrink-0 text-xs text-[var(--color-text-dim)]">
                  salt-okunur önizleme
                </span>
                <Button onClick={() => instantiateTemplate(selectedTemplate)} size="lg" className="flex-shrink-0">
                  + Bu şablondan akış oluştur
                </Button>
              </div>
              <div className="min-h-0 flex-1">
                <TemplatePreview graph={selectedTemplate.graph} agents={agents} />
              </div>
            </>
          )}
        </div>
      ) : tab === 'runs' ? (
        !selectedRun ? (
          <div className="flex-1 p-6">
            <p className="text-sm text-[var(--color-text-dim)]">
              Soldan bir koşu seçin — akışın hangi aşamada olduğunu, node çıktılarını ve
              hataları salt-okunur görün.
            </p>
          </div>
        ) : (
          <RunView
            run={selectedRun}
            flow={flows.find((f) => f.id === selectedRun.flowId)}
            agents={agents}
          />
        )
      ) : !selectedId ? (
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
            {/* Flow id + on-disk location (copy path / open folder). */}
            <span
              className="flex-shrink-0 rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 font-mono text-[11px] text-[var(--color-text-dim)]"
              title="Akış ID (dosya adı)"
            >
              {selectedId}
            </span>
            <CopyPathButton path={flowPath} title="Akış yolunu kopyala" />
            <button
              type="button"
              onClick={() => selectedId && api.revealFlow(selectedId).catch((e) => onError((e as Error).message))}
              title="Akış klasörünü aç"
              className="flex flex-shrink-0 items-center justify-center rounded border border-[var(--color-border)] px-1.5 py-1 text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
            >
              <FolderOpen size={14} />
            </button>
            <input
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="Açıklama — bu akış ne yapar? (isteğe bağlı)"
              className="min-w-0 flex-1 rounded bg-[var(--color-surface-2)] px-3 py-2 text-sm text-[var(--color-text-dim)] outline-none"
            />
            <label className="flex flex-shrink-0 items-center gap-1 text-xs text-[var(--color-text-dim)]">
              Kablo:
              <select
                value={edgeStyle}
                onChange={(e) => changeEdgeStyle(e.target.value as EdgeStyle)}
                className="rounded bg-[var(--color-surface-2)] px-2 py-1.5 text-xs outline-none"
              >
                {EDGE_STYLES.map((s) => (
                  <option key={s.value} value={s.value}>
                    {s.label}
                  </option>
                ))}
              </select>
            </label>
            <label className="flex flex-shrink-0 cursor-pointer items-center gap-1 text-xs text-[var(--color-text-dim)]">
              <input
                type="checkbox"
                checked={animated}
                onChange={(e) => setAnimated(e.target.checked)}
              />
              Animasyon
            </label>
            <Button onClick={saveFlow} size="lg" className="flex-shrink-0">
              Kaydet
            </Button>
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
                edgeStyle={edgeStyle}
                animated={animated}
                onNodesChange={onNodesChange}
                onEdgesChange={onEdgesChange}
                setEdges={setEdges}
                onSelect={setSelectedNodeId}
                onAutoLayout={autoArrange}
                nodeActions={{
                  onMakeStart: makeStartNode,
                  onDuplicate: duplicateNode,
                  onDelete: deleteNode,
                }}
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
              <Button onClick={doRun} disabled={running} size="lg">
                {running ? 'Çalışıyor…' : '▶ Çalıştır'}
              </Button>
            </div>

            {/* Live node progress while running (before the final run lands). */}
            {!run && liveNodes.length > 0 && (
              <div className="mt-4 border-t border-[var(--color-border)] pt-3">
                <div className="mb-2 text-xs text-[var(--color-text-dim)]">Canlı ilerleme</div>
                <ol className="space-y-2">
                  {liveNodes.map((n, i) => (
                    <li key={`${n.nodeId}-${i}`} className="rounded bg-[var(--color-surface-2)] p-2 text-sm">
                      <div className="mb-1 flex items-center gap-1.5 text-xs text-[var(--color-text-dim)]">
                        {n.error !== undefined ? (
                          <XCircle size={12} className="text-[var(--color-danger)]" />
                        ) : n.output === undefined ? (
                          <Loader2 size={12} className="animate-spin text-[var(--color-accent)]" />
                        ) : null}
                        {i + 1}. [{n.type}] {n.title}
                      </div>
                      {n.error !== undefined ? (
                        <span className="text-xs whitespace-pre-wrap text-[var(--color-danger)]">⚠️ {n.error}</span>
                      ) : n.output === undefined ? (
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
                  <span className={run.status === 'success' ? 'text-[var(--color-success)]' : 'text-[var(--color-danger)]'}>
                    {run.status}
                  </span>
                  {run.error && <span className="ml-2 text-[var(--color-danger)]">· {run.error}</span>}
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
