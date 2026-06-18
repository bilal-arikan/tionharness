import { useEffect, useMemo } from 'react'
import { useNodesState, useEdgesState, type Edge } from '@xyflow/react'
import { graphToReactFlow, type FlowRFNode, type NodeStatus } from '../../lib/flowGraph'
import type { Agent, Flow, FlowGraph, FlowRun, FlowState } from '../../types'
import { Markdown } from '../markdown/Markdown'
import { FlowCanvas } from './FlowCanvas'

interface Props {
  run: FlowRun
  flow: Flow | undefined
  agents: Agent[]
}

const STATUS_LABEL: Record<string, string> = {
  running: '▶ devam ediyor',
  success: '✓ başarılı',
  failure: '✕ hata',
}

function statusColor(status: string): string {
  if (status === 'success') return 'text-green-400'
  if (status === 'failure') return 'text-red-400'
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
export function RunView({ run, flow, agents }: Props) {
  const st = useMemo(() => safeParseState(run.state), [run.state])
  const graph = useMemo(() => (flow ? safeParseGraph(flow.graph) : null), [flow])
  const statuses = useMemo(() => nodeStatuses(run, st), [run, st])

  const [nodes, setNodes, onNodesChange] = useNodesState<FlowRFNode>([])
  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>([])

  useEffect(() => {
    if (!graph) {
      setNodes([])
      setEdges([])
      return
    }
    const { nodes: rn, edges: re } = graphToReactFlow(graph)
    setNodes(rn.map((n) => ({ ...n, data: { ...n.data, status: statuses[n.id] } })))
    setEdges(re)
  }, [graph, statuses, setNodes, setEdges])

  return (
    <div className="flex min-w-0 flex-1 flex-col">
      {/* Header */}
      <div className="border-b border-[var(--color-border)] p-3">
        <div className="flex items-center gap-2">
          <span className="truncate text-sm font-medium">{flow?.name ?? '（silinmiş akış）'}</span>
          <span className={`text-xs ${statusColor(run.status)}`}>
            {STATUS_LABEL[run.status] ?? run.status}
          </span>
          <span className="ml-auto text-xs text-[var(--color-text-dim)]">
            {new Date(run.createdAt * 1000).toLocaleString()}
          </span>
        </div>
        {run.input && (
          <div className="mt-1 truncate text-xs text-[var(--color-text-dim)]">
            Girdi: {run.input}
          </div>
        )}
        {run.error && (
          <div className="mt-1 whitespace-pre-wrap text-xs text-[var(--color-danger)]">
            ⚠️ {run.error}
          </div>
        )}
      </div>

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

      {/* Trace: node outputs + error */}
      <div className={`overflow-y-auto border-t border-[var(--color-border)] p-4 ${graph ? 'max-h-[40%]' : 'flex-1'}`}>
        <div className="mb-2 text-xs text-[var(--color-text-dim)]">Adım izi</div>
        <ol className="space-y-2">
          {(st?.trace ?? []).map((t, i) => (
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
          {(st?.trace ?? []).length === 0 && (
            <li className="text-xs italic text-[var(--color-text-dim)]">
              {run.status === 'running' ? 'Henüz adım tamamlanmadı…' : 'Adım izi yok.'}
            </li>
          )}
        </ol>
      </div>
    </div>
  )
}
