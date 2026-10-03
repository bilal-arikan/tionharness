// Pure graph helpers for the Flows screen: parsing, defaults, auto layout,
// React Flow conversion and a cheap client-side lint. The engine's real
// validation runs on the backend (POST /api/flows/{id}/validate, PUT save).
import type { Edge, Node } from '@xyflow/react'
import type { FlowEdge, FlowGraph, FlowNode, FlowNodeType } from '@/types'

export type NodeStatus = 'running' | 'done' | 'error'

export interface FlowNodeData extends Record<string, unknown> {
  node: FlowNode
  status?: NodeStatus
  // Where a route node's outgoing arms go (label → target id), for the card body.
  arms?: { when: string; to: string }[]
  agentName?: string
  automationName?: string
}

export type FlowRFNode = Node<FlowNodeData, 'flow'>
export type FlowRFEdge = Edge<{ when?: string; isRoute: boolean }>

export const NODE_W = 220
const COL_GAP = 60
const ROW_GAP = 120

export function emptyGraph(): FlowGraph {
  return { version: 2, nodes: [], edges: [] }
}

export function parseGraph(raw: string): FlowGraph {
  try {
    const g = JSON.parse(raw) as Partial<FlowGraph>
    return {
      version: 2,
      nodes: Array.isArray(g.nodes) ? g.nodes : [],
      edges: Array.isArray(g.edges) ? g.edges : [],
      maxSteps: g.maxSteps,
    }
  } catch {
    return emptyGraph()
  }
}

// canonicalKey is the dirty-detection key: it ignores node positions, which are
// cosmetic and saved without a version bump.
export function canonicalKey(g: FlowGraph): string {
  const nodes = [...g.nodes]
    .map((n) => {
      const { x: _x, y: _y, ...rest } = n
      return rest
    })
    .sort((a, b) => a.id.localeCompare(b.id))
  const edges = [...g.edges].sort((a, b) => a.id.localeCompare(b.id))
  return JSON.stringify({ nodes, edges, maxSteps: g.maxSteps ?? 0 })
}

export function layoutKey(g: FlowGraph): string {
  return JSON.stringify(g.nodes.map((n) => [n.id, Math.round(n.x ?? 0), Math.round(n.y ?? 0)]))
}

export function freshNodeId(g: FlowGraph, base: string): string {
  const taken = new Set(g.nodes.map((n) => n.id))
  if (!taken.has(base)) return base
  for (let i = 2; ; i++) {
    const cand = `${base}_${i}`
    if (!taken.has(cand)) return cand
  }
}

export function freshEdgeId(g: FlowGraph, from: string, to: string): string {
  const taken = new Set(g.edges.map((e) => e.id))
  const base = `e_${from}_${to}`
  if (!taken.has(base)) return base
  for (let i = 2; ; i++) {
    const cand = `${base}_${i}`
    if (!taken.has(cand)) return cand
  }
}

export function defaultNode(type: FlowNodeType, id: string, title: string): FlowNode {
  switch (type) {
    case 'llm':
      return { id, type, title, prompt: '{{input}}', context: 'thread', tools: 'inherit' }
    case 'route':
      return { id, type, title, mode: 'contains', maxVisits: 3 }
    case 'transform':
      return { id, type, title, template: '{{last}}' }
    case 'trigger':
      return { id, type, title, template: '{{last}}' }
    case 'output':
      return { id, type, title, template: '{{last}}' }
    default:
      return { id, type, title }
  }
}

export function outgoing(g: FlowGraph, id: string): FlowEdge[] {
  return g.edges.filter((e) => e.from === id)
}

// autoLayout places nodes top-to-bottom by BFS depth from the input node; a
// back edge (loop) does not pull its target down. Nodes the walk never reaches
// are stacked below. Returns a new graph; the input is not mutated.
export function autoLayout(g: FlowGraph): FlowGraph {
  const depth = new Map<string, number>()
  const input = g.nodes.find((n) => n.type === 'input')
  const order: string[] = []
  if (input) {
    const queue: string[] = [input.id]
    depth.set(input.id, 0)
    while (queue.length) {
      const cur = queue.shift()!
      order.push(cur)
      for (const e of outgoing(g, cur)) {
        if (!depth.has(e.to)) {
          depth.set(e.to, (depth.get(cur) ?? 0) + 1)
          queue.push(e.to)
        }
      }
    }
  }
  // The output node sits at the bottom even when a short branch reaches it early.
  const maxDepth = Math.max(0, ...[...depth.values()])
  for (const n of g.nodes) {
    if (n.type === 'output' && depth.has(n.id))
      depth.set(n.id, Math.max(depth.get(n.id)!, maxDepth))
  }
  let extra = maxDepth + 1
  for (const n of g.nodes) {
    if (!depth.has(n.id)) depth.set(n.id, extra++)
  }
  const rows = new Map<number, string[]>()
  for (const n of g.nodes) {
    const d = depth.get(n.id) ?? 0
    rows.set(d, [...(rows.get(d) ?? []), n.id])
  }
  const pos = new Map<string, { x: number; y: number }>()
  for (const [d, ids] of rows) {
    const width = ids.length * NODE_W + (ids.length - 1) * COL_GAP
    ids.forEach((id, i) => {
      pos.set(id, { x: -width / 2 + i * (NODE_W + COL_GAP), y: d * ROW_GAP * 1.4 })
    })
  }
  return {
    ...g,
    nodes: g.nodes.map((n) => ({ ...n, ...(pos.get(n.id) ?? { x: n.x ?? 0, y: n.y ?? 0 }) })),
  }
}

// needsLayout reports whether every node still sits at the origin (a graph
// written by the backend or an agent, which never positions nodes).
export function needsLayout(g: FlowGraph): boolean {
  if (g.nodes.length <= 1) return false
  const seen = new Set<string>()
  for (const n of g.nodes) {
    const key = `${Math.round(n.x ?? 0)}:${Math.round(n.y ?? 0)}`
    if (seen.has(key)) return true
    seen.add(key)
  }
  return false
}

export function toReactFlow(
  g: FlowGraph,
  statuses: Map<string, NodeStatus>,
  agentNames: Map<string, string>,
  automationNames: Map<string, string> = new Map(),
): { nodes: FlowRFNode[]; edges: FlowRFEdge[] } {
  const nodes: FlowRFNode[] = g.nodes.map((n) => ({
    id: n.id,
    type: 'flow',
    position: { x: n.x ?? 0, y: n.y ?? 0 },
    data: {
      node: n,
      status: statuses.get(n.id),
      arms:
        n.type === 'route'
          ? outgoing(g, n.id).map((e) => ({ when: e.when ?? '', to: e.to }))
          : undefined,
      agentName: n.agentId ? agentNames.get(n.agentId) : undefined,
      automationName: n.automationId ? automationNames.get(n.automationId) : undefined,
    },
    draggable: true,
  }))
  const byId = new Map(g.nodes.map((n) => [n.id, n]))
  const edges: FlowRFEdge[] = g.edges.map((e) => ({
    id: e.id,
    source: e.from,
    target: e.to,
    type: 'flow',
    data: { when: e.when, isRoute: byId.get(e.from)?.type === 'route' },
  }))
  return { nodes, edges }
}

// applyPositions writes the canvas positions back into the graph.
export function applyPositions(g: FlowGraph, nodes: FlowRFNode[]): FlowGraph {
  const pos = new Map(nodes.map((n) => [n.id, n.position]))
  return {
    ...g,
    nodes: g.nodes.map((n) => {
      const p = pos.get(n.id)
      return p ? { ...n, x: Math.round(p.x), y: Math.round(p.y) } : n
    }),
  }
}

// lint runs the structural checks that need no backend: counts, dangling edges,
// linear fan-out. Messages are i18n keys with params.
export function lint(g: FlowGraph): { key: string; params?: Record<string, unknown> }[] {
  const out: { key: string; params?: Record<string, unknown> }[] = []
  const ids = new Set(g.nodes.map((n) => n.id))
  const inputs = g.nodes.filter((n) => n.type === 'input').length
  const outputs = g.nodes.filter((n) => n.type === 'output').length
  if (inputs !== 1) out.push({ key: 'lint.oneInput', params: { count: inputs } })
  if (outputs !== 1) out.push({ key: 'lint.oneOutput', params: { count: outputs } })
  for (const e of g.edges) {
    if (!ids.has(e.from) || !ids.has(e.to))
      out.push({ key: 'lint.danglingEdge', params: { id: e.id } })
  }
  for (const n of g.nodes) {
    const outs = outgoing(g, n.id).length
    if (
      (n.type === 'llm' || n.type === 'transform' || n.type === 'trigger' || n.type === 'input') &&
      outs !== 1
    ) {
      out.push({ key: 'lint.oneOutgoing', params: { id: n.id, count: outs } })
    }
    if (n.type === 'route' && outs === 0) out.push({ key: 'lint.routeNoArm', params: { id: n.id } })
    if (n.type === 'route' && n.mode === 'criteria') {
      if (!(n.criteria ?? []).some((c) => c.trim()))
        out.push({ key: 'lint.criteriaEmpty', params: { id: n.id } })
      const labels = outgoing(g, n.id).map((e) => (e.when ?? '').trim().toLowerCase())
      if (!labels.includes('pass') && !labels.includes('fail'))
        out.push({ key: 'lint.criteriaArms', params: { id: n.id } })
    }
    if (n.type === 'trigger' && !n.automationId?.trim())
      out.push({ key: 'lint.triggerAutomation', params: { id: n.id } })
    if (n.type === 'output' && outs !== 0)
      out.push({ key: 'lint.outputNoOutgoing', params: { id: n.id } })
    if (n.type === 'transform' && !n.template?.trim())
      out.push({ key: 'lint.transformTemplate', params: { id: n.id } })
  }
  return out
}

// shapeOf renders the one-line summary the backend also produces, so the list
// row and the inspector agree before a save.
export function shapeOf(g: FlowGraph): string {
  const input = g.nodes.find((n) => n.type === 'input')
  if (!input) return `${g.nodes.length} nodes`
  const seen = new Set<string>()
  const parts: string[] = []
  let cur: string | undefined = input.id
  while (cur && !seen.has(cur)) {
    seen.add(cur)
    const n = g.nodes.find((x) => x.id === cur)
    if (!n) break
    if (n.type === 'input' || n.type === 'output') parts.push(n.type)
    else if (n.type === 'route') {
      const arms = outgoing(g, n.id).map((e) => `${e.when || '*'}→${e.to}`)
      parts.push(`${n.id}(route: ${arms.join(', ')})`)
    } else parts.push(`${n.id}(${n.type})`)
    const outs = outgoing(g, n.id)
    cur = outs.find((e) => !seen.has(e.to))?.to
  }
  for (const n of g.nodes) if (!seen.has(n.id)) parts.push(`${n.id}(${n.type})`)
  return parts.join(' → ')
}
