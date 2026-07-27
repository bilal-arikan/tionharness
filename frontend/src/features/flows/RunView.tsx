import { useEffect, useMemo, useState } from 'react'
import { useNodesState, useEdgesState, type Edge } from '@xyflow/react'
import { RotateCcw, ChevronDown, ChevronUp, Send, Loader2 } from 'lucide-react'
import { api } from '@/api'
import { graphToReactFlow, type FlowRFNode, type NodeStatus } from './flowGraph'
import type { Agent, Flow, FlowGraph, FlowNodeEvent, FlowRun, FlowState } from '@/types'
import { Markdown } from '@/shared/components/markdown/Markdown'
import { normalizeAvatar } from '@/shared/lib/avatar'
import { subscribeFlowNode } from '@/shared/lib/flowNodeBus'
import { FlowCanvas } from './FlowCanvas'
import { RunNodeInspector } from './RunNodeInspector'

interface Props {
  run: FlowRun
  flow: Flow | undefined
  agents: Agent[]
  // Re-run this run's flow with the SAME input (Koşular tab). Absent → no button.
  onRerun?: (run: FlowRun) => void
  // True while a re-run kicked off from this view is in flight.
  rerunning?: boolean
  // Hide the top summary row (flow name + status + date + rerun) when the parent
  // lifts it into the screen's top bar (PaneHeader). Input/error rows still show.
  hideSummary?: boolean
  // Called right after input is delivered to a waiting run, so the parent can
  // refresh the runs list without waiting for the next poll.
  onResumed?: () => void
  // Move the run's input out of the top detail row and into the step-trace panel
  // as its first "Girdi" entry (chat flow view: the input reads inline with the
  // steps instead of a separate header row). Koşular keeps the top row.
  inputInTrace?: boolean
}

export const STATUS_LABEL: Record<string, string> = {
  running: '▶ devam ediyor',
  success: '✓ başarılı',
  failure: '✕ hata',
  waiting: '⏳ girdi bekleniyor',
}

export function statusColor(status: string): string {
  if (status === 'success') return 'text-[var(--color-success)]'
  if (status === 'failure') return 'text-[var(--color-danger)]'
  if (status === 'waiting') return 'text-[#eab308]'
  return 'text-[var(--color-accent)]'
}

function safeParseState(s: string): FlowState | null {
  try {
    return JSON.parse(s) as FlowState
  } catch {
    return null
  }
}

function safeParseGraph(s: string): FlowGraph | null {
  try {
    return JSON.parse(s) as FlowGraph
  } catch {
    return null
  }
}

// nodeStatuses derives per-node run status from the persisted flow state:
// executed (trace) nodes are "done"; the current node is "running" (live) or
// "error" (on failure, the engine leaves Current pointing at the failing node).
function nodeStatuses(run: FlowRun, st: FlowState | null): Record<string, NodeStatus> {
  const map: Record<string, NodeStatus> = {}
  if (!st) return map
  for (const t of st.trace ?? []) map[t.nodeId] = 'done'
  if (run.status === 'waiting' && st.waitingAt) {
    map[st.waitingAt] = 'waiting'
  } else if (st.current) {
    if (run.status === 'running') map[st.current] = 'running'
    else if (run.status === 'failure') map[st.current] = 'error'
  }
  return map
}

// RunView is the read-only inspector for a single flow run: a non-interactive
// canvas annotated with per-node run status (which stage we're at), plus the
// node-by-node trace with outputs and any error.
export function RunView({ run, flow, agents, onRerun, rerunning, hideSummary, onResumed, inputInTrace }: Props) {
  const st = useMemo(() => safeParseState(run.state), [run.state])
  // Await-input composer state (only used while the run is waiting).
  const [awaitInput, setAwaitInput] = useState('')
  const [delivering, setDelivering] = useState(false)
  const deliverInput = async () => {
    if (delivering) return
    setDelivering(true)
    try {
      await api.resumeFlowRun(run.id, awaitInput)
      setAwaitInput('')
      onResumed?.()
    } catch {
      // 409 = already resumed by another window; the poll will reconcile.
    } finally {
      setDelivering(false)
    }
  }
  const graph = useMemo(() => (flow ? safeParseGraph(flow.graph) : null), [flow])
  const statuses = useMemo(() => nodeStatuses(run, st), [run, st])

  // Clicking a canvas node opens its chat-like inspector in the bottom panel
  // (input bubble + steps + output) instead of the flat all-nodes list. Cleared
  // on run switch and when the flow/node no longer exists.
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null)
  useEffect(() => setSelectedNodeId(null), [run.id])

  // Live per-node frames off the flow-node bus (keyed by run id). They render
  // node start/done/error + output the instant the engine emits it — ahead of
  // the parent's ~3s run-state poll, and for autonomous/scheduled runs that have
  // no per-request SSE at all. Latest frame per node; cleared on run switch.
  const [live, setLive] = useState<Record<string, FlowNodeEvent>>({})
  useEffect(() => {
    setLive({})
    return subscribeFlowNode(run.id, (ev) => {
      setLive((prev) => ({ ...prev, [ev.nodeId]: ev }))
    })
  }, [run.id])

  // Merge live statuses onto the persisted ones, never regressing: a node only
  // advances (pending → running → done/error), so a live "done" is not undone by
  // a stale poll still calling the node "running".
  const rank: Record<NodeStatus, number> = { waiting: 1, running: 1, done: 2, error: 2 }
  const mergedStatuses = useMemo(() => {
    const merged: Record<string, NodeStatus> = { ...statuses }
    for (const ev of Object.values(live)) {
      const s: NodeStatus =
        ev.phase === 'done'
          ? 'done'
          : ev.phase === 'error'
            ? 'error'
            : ev.phase === 'waiting'
              ? 'waiting'
              : 'running'
      const cur = merged[ev.nodeId]
      if (!cur || rank[s] > rank[cur]) merged[ev.nodeId] = s
    }
    return merged
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [live, statuses])

  // Trace = persisted entries + any live "done" nodes the poll hasn't recorded
  // yet (appended in execution order), so a finished node's output shows at once.
  const liveTrace = useMemo(() => {
    const base = st?.trace ?? []
    const seen = new Set(base.map((t) => t.nodeId))
    const extras = Object.values(live)
      .filter((ev) => ev.phase === 'done' && !seen.has(ev.nodeId))
      .sort((a, b) => a.index - b.index)
      .map((ev) => ({ nodeId: ev.nodeId, type: ev.type, title: ev.title, output: ev.output ?? '', at: 0 }))
    return [...base, ...extras]
  }, [st, live])

  // The node currently executing (a live "start" with no matching "done"/trace),
  // rendered as a trailing "running" row so the panel isn't silent mid-node.
  const runningNode = useMemo(() => {
    const done = new Set([
      ...(st?.trace ?? []).map((t) => t.nodeId),
      ...Object.values(live)
        .filter((e) => e.phase === 'done' || e.phase === 'error')
        .map((e) => e.nodeId),
    ])
    // A 'progress' frame (e.g. a join barrier reporting "2/5") keeps the node in
    // the running row too — its latest frame is progress, not start.
    return Object.values(live).find((ev) => (ev.phase === 'start' || ev.phase === 'progress') && !done.has(ev.nodeId)) ?? null
  }, [st, live])

  // Collapsible "Adım izi" (step trace) bottom panel. Persisted; defaults open on
  // wide screens but CLOSED on narrow (< md) ones, where it otherwise squeezes the
  // canvas and breaks the vertical layout.
  const [traceOpen, setTraceOpen] = useState(() => {
    const v = localStorage.getItem('tionswarm.flowTraceOpen')
    if (v !== null) return v !== '0'
    return typeof window === 'undefined' || window.innerWidth >= 768
  })
  const toggleTrace = () =>
    setTraceOpen((o) => {
      const next = !o
      localStorage.setItem('tionswarm.flowTraceOpen', next ? '1' : '0')
      return next
    })
  const traceCount = liveTrace.length

  // The selected node (graph def) + its trace entry (input/output), feeding the
  // NodeInspector chat view. A selection for a node not in the graph (deleted
  // flow) or not yet executed collapses back to the flat list.
  const selectedNode = useMemo(
    () => (selectedNodeId && graph ? (graph.nodes.find((n) => n.id === selectedNodeId) ?? null) : null),
    [selectedNodeId, graph],
  )
  const selectedEntry = useMemo(
    () => (selectedNodeId ? liveTrace.find((t) => t.nodeId === selectedNodeId) : undefined),
    [selectedNodeId, liveTrace],
  )
  // Lookup for the parallel fan-out view (child id → its trace entry). Last write
  // wins if a node executed more than once (loop) — the latest output.
  const traceByNode = useMemo(() => {
    const m: Record<string, (typeof liveTrace)[number]> = {}
    for (const t of liveTrace) m[t.nodeId] = t
    return m
  }, [liveTrace])

  const [nodes, setNodes, onNodesChange] = useNodesState<FlowRFNode>([])
  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>([])

  useEffect(() => {
    if (!graph) {
      setNodes([])
      setEdges([])
      return
    }
    const { nodes: rn, edges: re } = graphToReactFlow(graph)
    // Per-node output: prefer the live "done" frame, else the persisted trace, so
    // a finished node can render its reply inline on the canvas (AgentNode).
    const outputs: Record<string, string> = {}
    for (const t of liveTrace) outputs[t.nodeId] = t.output
    // Preserve in-session drag positions AND the current selection: the ~3s
    // status/output poll re-runs this effect, so rebuilding straight from the
    // graph would (a) snap a just-dragged node back and (b) drop React Flow's
    // `selected` flag — which fires onSelectionChange(empty) and closes the open
    // node inspector. Carry both over for nodes that already exist.
    setNodes((prev) => {
      const byId = new Map(prev.map((n) => [n.id, n]))
      return rn.map((n) => {
        const p = byId.get(n.id)
        return {
          ...n,
          position: p?.position ?? n.position,
          selected: p?.selected,
          data: { ...n.data, status: mergedStatuses[n.id], output: outputs[n.id] },
        }
      })
    })
    setEdges(re)
  }, [graph, mergedStatuses, liveTrace, setNodes, setEdges])

  // Persist a tidied layout: dragging a node in the run inspector writes its new
  // x/y back to the flow definition (the run renders the live flow graph, not a
  // snapshot, so the layout is shared with the editor). Only the moved node's
  // position changes; everything else in the graph is left intact.
  const persistNodePosition = async (id: string, pos: { x: number; y: number }) => {
    if (!flow || !graph) return
    const nextNodes = graph.nodes.map((n) =>
      n.id === id ? { ...n, x: Math.round(pos.x), y: Math.round(pos.y) } : n,
    )
    try {
      await api.updateFlow(flow.id, flow.name, { ...graph, nodes: nextNodes })
    } catch (e) {
      // Cosmetic-only; surface for debugging but don't disrupt the inspector.
      console.error('flow layout save failed', e)
    }
  }

  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col">
      {/* Header: run summary (flow name + status + date + rerun) plus input/error
          detail rows. The summary row is hidden when the parent lifts it into the
          screen's top bar (PaneHeader); the input/error rows always show here. */}
      {(!hideSummary || (run.input && !inputInTrace) || run.error) && (
        <div className="border-b border-[var(--color-border)] p-3">
          {!hideSummary && (
            <div className="flex items-center gap-2">
              <span className="truncate text-sm font-medium">
                {normalizeAvatar(flow?.emoji) && (
                  <span className="mr-1 leading-none">{normalizeAvatar(flow?.emoji)}</span>
                )}
                {flow?.name ?? '（silinmiş akış）'}
              </span>
              <span className={`text-xs ${statusColor(run.status)}`}>
                {STATUS_LABEL[run.status] ?? run.status}
              </span>
              <span className="ml-auto text-xs text-[var(--color-text-dim)]">
                {new Date(run.createdAt * 1000).toLocaleString()}
              </span>
              {onRerun && (
                <button
                  type="button"
                  onClick={() => onRerun(run)}
                  disabled={rerunning || run.status === 'running' || !flow}
                  title={
                    !flow
                      ? 'Akış silinmiş — tekrar çalıştırılamaz'
                      : 'Bu koşuyu aynı girdiyle tekrar çalıştır'
                  }
                  className="flex shrink-0 items-center gap-1.5 rounded-md border border-[var(--color-border)] px-2.5 py-1 text-xs text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)] disabled:cursor-not-allowed disabled:opacity-50"
                >
                  <RotateCcw size={13} className={rerunning ? 'animate-spin' : ''} />
                  {rerunning ? 'Çalışıyor…' : 'Tekrar çalıştır'}
                </button>
              )}
            </div>
          )}
          {run.input && !inputInTrace && (
            <div className={`${hideSummary ? '' : 'mt-1 '}truncate text-xs text-[var(--color-text-dim)]`}>
              Girdi: {run.input}
            </div>
          )}
          {run.error && (
            <div className={`${hideSummary ? '' : 'mt-1 '}whitespace-pre-wrap text-xs text-[var(--color-danger)]`}>
              ⚠️ {run.error}
            </div>
          )}
        </div>
      )}

      {/* Await-input composer: the run paused at an await-input node and needs
          input to continue. Delivering resumes it (any window/peer can); the poll
          then reflects the run advancing. */}
      {run.status === 'waiting' && (
        <div className="flex items-center gap-2 border-b border-[color:#eab308] bg-[color:color-mix(in_srgb,#eab308_10%,var(--color-surface))] p-3">
          <span className="shrink-0 text-xs text-[#eab308]">⏳ Girdi bekleniyor</span>
          <input
            value={awaitInput}
            onChange={(e) => setAwaitInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault()
                void deliverInput()
              }
            }}
            placeholder="Akışa gönderilecek girdi…"
            autoFocus
            className="min-w-0 flex-1 rounded bg-[var(--color-surface-2)] px-2 py-1.5 text-sm outline-none"
          />
          <button
            onClick={() => void deliverInput()}
            disabled={delivering}
            className="flex shrink-0 items-center gap-1.5 rounded-md bg-[#eab308] px-3 py-1.5 text-xs font-medium text-black transition hover:opacity-90 disabled:opacity-50"
          >
            {delivering ? <Loader2 size={13} className="animate-spin" /> : <Send size={13} />}
            Gönder
          </button>
        </div>
      )}

      {/* Read-only canvas with per-node status (only if the flow still exists) */}
      {graph && (
        <div className="min-h-0 flex-1">
          <FlowCanvas
            agents={agents}
            nodes={nodes}
            edges={edges}
            edgeStyle={(graph.edgeStyle as never) || 'default'}
            animated={!!graph.animated}
            onNodesChange={onNodesChange}
            onEdgesChange={onEdgesChange}
            setEdges={setEdges}
            onSelect={(id) => {
              setSelectedNodeId(id)
              if (id) setTraceOpen(true)
            }}
            runMode
            onNodeDragStop={(id, pos) => void persistNodePosition(id, pos)}
          />
        </div>
      )}

      {/* Trace: node outputs + error. A collapsible bottom panel — the header is a
          toggle button; when open it expands upward (capped) with its own scroll,
          when closed only the header bar remains so the canvas keeps the height. */}
      <div className={`flex flex-col border-t border-[var(--color-border)] ${!graph && traceOpen ? 'min-h-0 flex-1' : ''}`}>
        <button
          type="button"
          onClick={toggleTrace}
          aria-expanded={traceOpen}
          className="flex flex-shrink-0 items-center gap-1 px-4 py-2 text-xs text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
          title={traceOpen ? 'Adım izini gizle' : 'Adım izini göster'}
        >
          {traceOpen ? <ChevronDown size={13} /> : <ChevronUp size={13} />}
          <span>{selectedNode ? 'Node görünümü' : 'Adım izi'}</span>
          <span className="ml-auto opacity-70">
            {selectedNode ? (selectedNode.title || selectedNode.id) : `${traceCount} adım`}
          </span>
        </button>
        {traceOpen && selectedNode ? (
          <div className={`flex min-h-0 flex-col ${graph ? 'max-h-[40vh]' : 'flex-1'}`}>
            <RunNodeInspector
              run={run}
              node={selectedNode}
              entry={selectedEntry}
              status={mergedStatuses[selectedNode.id]}
              thread={st?.thread}
              traceByNode={traceByNode}
              agents={agents}
              onClose={() => setSelectedNodeId(null)}
              onSelectNode={(id) => setSelectedNodeId(id)}
            />
          </div>
        ) : traceOpen ? (
          <div className={`overflow-y-auto px-4 pb-4 ${graph ? 'max-h-[40vh]' : 'min-h-0 flex-1'}`}>
            <ol className="space-y-2">
              {/* Run input as the first step-trace entry (chat flow view): the
                  girdi reads inline with the steps instead of a top header row. */}
              {inputInTrace && run.input && (
                <li className="rounded border-l-2 border-[var(--color-accent)] bg-[var(--color-surface-2)] p-2 text-sm">
                  <div className="mb-1 text-xs text-[var(--color-text-dim)]">Girdi</div>
                  <div className="whitespace-pre-wrap">{run.input}</div>
                </li>
              )}
              {liveTrace.map((t, i) => (
                <li key={`${t.nodeId}-${i}`} className="rounded bg-[var(--color-surface-2)] p-2 text-sm">
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
              {/* Currently-executing node (live "start" with no output yet). */}
              {runningNode && (
                <li className="rounded bg-[var(--color-surface-2)] p-2 text-sm">
                  <div className="text-xs text-[var(--color-accent)]">
                    {traceCount + 1}. [{runningNode.type}] {runningNode.title} —{' '}
                    {runningNode.phase === 'progress' && runningNode.output
                      ? `${runningNode.output} tamamlandı…`
                      : 'çalışıyor…'}
                  </div>
                </li>
              )}
              {traceCount === 0 && !runningNode && (
                <li className="text-xs italic text-[var(--color-text-dim)]">
                  {run.status === 'running' ? 'Henüz adım tamamlanmadı…' : 'Adım izi yok.'}
                </li>
              )}
            </ol>
          </div>
        ) : null}
      </div>
    </div>
  )
}
