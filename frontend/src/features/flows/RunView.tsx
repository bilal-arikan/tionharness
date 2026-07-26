import { useEffect, useMemo, useState } from 'react'
import { useNodesState, useEdgesState, type Edge } from '@xyflow/react'
import { RotateCcw, ChevronDown, ChevronUp } from 'lucide-react'
import { graphToReactFlow, type FlowRFNode, type NodeStatus } from './flowGraph'
import type { Agent, Flow, FlowGraph, FlowNodeEvent, FlowRun, FlowState } from '@/types'
import { Markdown } from '@/shared/components/markdown/Markdown'
import { normalizeAvatar } from '@/shared/lib/avatar'
import { subscribeFlowNode } from '@/shared/lib/flowNodeBus'
import { FlowCanvas } from './FlowCanvas'

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
}

export const STATUS_LABEL: Record<string, string> = {
  running: '▶ devam ediyor',
  success: '✓ başarılı',
  failure: '✕ hata',
}

export function statusColor(status: string): string {
  if (status === 'success') return 'text-[var(--color-success)]'
  if (status === 'failure') return 'text-[var(--color-danger)]'
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
  if (st.current) {
    if (run.status === 'running') map[st.current] = 'running'
    else if (run.status === 'failure') map[st.current] = 'error'
  }
  return map
}

// RunView is the read-only inspector for a single flow run: a non-interactive
// canvas annotated with per-node run status (which stage we're at), plus the
// node-by-node trace with outputs and any error.
export function RunView({ run, flow, agents, onRerun, rerunning, hideSummary }: Props) {
  const st = useMemo(() => safeParseState(run.state), [run.state])
  const graph = useMemo(() => (flow ? safeParseGraph(flow.graph) : null), [flow])
  const statuses = useMemo(() => nodeStatuses(run, st), [run, st])

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
  const rank: Record<NodeStatus, number> = { running: 1, done: 2, error: 2 }
  const mergedStatuses = useMemo(() => {
    const merged: Record<string, NodeStatus> = { ...statuses }
    for (const ev of Object.values(live)) {
      const s: NodeStatus = ev.phase === 'done' ? 'done' : ev.phase === 'error' ? 'error' : 'running'
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
    return Object.values(live).find((ev) => ev.phase === 'start' && !done.has(ev.nodeId)) ?? null
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
    setNodes(rn.map((n) => ({ ...n, data: { ...n.data, status: mergedStatuses[n.id], output: outputs[n.id] } })))
    setEdges(re)
  }, [graph, mergedStatuses, liveTrace, setNodes, setEdges])

  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col">
      {/* Header: run summary (flow name + status + date + rerun) plus input/error
          detail rows. The summary row is hidden when the parent lifts it into the
          screen's top bar (PaneHeader); the input/error rows always show here. */}
      {(!hideSummary || run.input || run.error) && (
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
          {run.input && (
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
            onSelect={() => {}}
            readOnly
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
          <span>Adım izi</span>
          <span className="ml-auto opacity-70">{traceCount} adım</span>
        </button>
        {traceOpen && (
          <div className={`overflow-y-auto px-4 pb-4 ${graph ? 'max-h-[40vh]' : 'min-h-0 flex-1'}`}>
            <ol className="space-y-2">
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
                    {traceCount + 1}. [{runningNode.type}] {runningNode.title} — çalışıyor…
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
        )}
      </div>
    </div>
  )
}
