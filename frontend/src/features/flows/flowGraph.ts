// Adapter between TionSwarm's orchestration graph (FlowGraph: start + nodes with
// next/branches/parallel/joinNext) and React Flow's nodes+edges model. The
// FlowNode itself is carried as RFNode.data so custom node components and the
// inspector edit it directly; edges are derived from the routing fields.
import type { Edge, Node as RFNode } from '@xyflow/react'
import type { FlowGraph, FlowNode, FlowNodeType } from '@/types'

export type FlowRFNode = RFNode<{
  node: FlowNode
  isStart: boolean
  status?: NodeStatus
  // A finished node's output (run views only) → rendered as an inline preview on
  // the node when status is "done". Undefined in the editor (no run outputs).
  output?: string
}>
export type NodeStatus = 'running' | 'done' | 'error' | 'waiting'

// Layout grid spacing for auto-placed nodes. Kept tight (a node is ≤220px wide)
// so an auto-arranged graph stays compact and readable without much panning.
const COL_W = 230
// Vertical gap between layered rows. Just clears a node with a prompt + 3-line
// output preview (run/session-flow views); tighter would risk overlap.
const ROW_H = 120

// edgeId builds a stable id for a routing edge. `slot` distinguishes a branch's
// multiple outgoing edges (one per arm) so they don't collide.
function edgeId(source: string, target: string, slot = ''): string {
  return `e:${source}:${slot}->${target}`
}

// graphToReactFlow converts a stored FlowGraph into React Flow nodes + edges.
// Nodes without persisted x/y are positioned by autoLayout.
export function graphToReactFlow(graph: FlowGraph): { nodes: FlowRFNode[]; edges: Edge[] } {
  const positions = needsLayout(graph) ? autoLayout(graph) : {}
  const nodes: FlowRFNode[] = graph.nodes.map((n) => ({
    id: n.id,
    type: n.type,
    position: { x: n.x ?? positions[n.id]?.x ?? 0, y: n.y ?? positions[n.id]?.y ?? 0 },
    data: { node: n, isStart: n.type === 'start' },
  }))

  const edges: Edge[] = []
  const add = (source: string, target: string, opts: Partial<Edge> & { slot?: string } = {}) => {
    if (!target) return // "" = end
    const { slot, ...rest } = opts
    edges.push({ id: edgeId(source, target, slot), source, target, ...rest })
  }

  for (const n of graph.nodes) {
    switch (n.type) {
      case 'agent':
      case 'delay':
      case 'transform':
      case 'await-input':
      case 'subflow':
      case 'start':
      case 'spawn':
      case 'join':
      case 'coordinator':
        add(n.id, n.next ?? '')
        break
      case 'end':
        break // terminal — no outgoing edge
      case 'branch':
        (n.branches ?? []).forEach((b, i) =>
          add(n.id, b.next, {
            slot: `b${i}`,
            sourceHandle: `b${i}`,
            label: b.contains || 'varsayılan',
            animated: false,
          }),
        )
        break
      case 'parallel':
        (n.parallel ?? []).forEach((childId) =>
          add(n.id, childId, { slot: 'fan', sourceHandle: 'fan' }),
        )
        add(n.id, n.joinNext ?? '', { slot: 'join', sourceHandle: 'join', label: 'join' })
        break
      case 'loop':
        add(n.id, n.body ?? '', { slot: 'body', sourceHandle: 'body', label: 'gövde' })
        add(n.id, n.loopNext ?? '', { slot: 'loop', sourceHandle: 'loop', label: 'çıkış' })
        break
    }
  }
  return { nodes, edges }
}

// reactFlowToGraph rebuilds a FlowGraph from canvas nodes + edges, reading the
// routing back out of the edges (by source handle) and persisting positions.
export function reactFlowToGraph(
  nodes: FlowRFNode[],
  edges: Edge[],
  start: string,
): FlowGraph {
  const out: FlowNode[] = nodes.map((rn) => {
    const base: FlowNode = { ...rn.data.node, x: round(rn.position.x), y: round(rn.position.y) }
    const outgoing = edges.filter((e) => e.source === rn.id)
    switch (base.type) {
      case 'agent':
      case 'delay':
      case 'transform':
      case 'await-input':
      case 'subflow':
      case 'start':
      case 'spawn':
      case 'join':
      case 'coordinator':
        base.next = outgoing[0]?.target ?? ''
        break
      case 'end':
        break // terminal — no next
      case 'branch': {
        // Keep existing arm conditions, re-target by branch slot order.
        const arms = base.branches ?? []
        base.branches = arms.map((b, i) => {
          const e = outgoing.find((x) => x.sourceHandle === `b${i}`)
          return { contains: b.contains, next: e?.target ?? '' }
        })
        break
      }
      case 'parallel':
        base.parallel = outgoing.filter((e) => e.sourceHandle === 'fan').map((e) => e.target)
        base.joinNext = outgoing.find((e) => e.sourceHandle === 'join')?.target ?? ''
        break
      case 'loop':
        base.body = outgoing.find((e) => e.sourceHandle === 'body')?.target ?? ''
        base.loopNext = outgoing.find((e) => e.sourceHandle === 'loop')?.target ?? ''
        break
    }
    return base
  })
  return { start, nodes: out }
}

// canonicalGraphKey returns a stable string identity for a FlowGraph, invariant
// to representational differences: the backend's `omitempty` marshaling drops
// next:""/x/y:0/empty-prompt, and JSON key order isn't guaranteed. It re-runs the
// editor's own graphToReactFlow → reactFlowToGraph round-trip so a stored graph
// and the live canvas reconstruction collapse to the same key when structurally
// equal. Cosmetic fields (edgeStyle/animated) are intentionally excluded, so they
// never raise a false "unsaved edits" signal. Use for dirty-checking.
export function canonicalGraphKey(graph: FlowGraph): string {
  const { nodes, edges } = graphToReactFlow({
    start: graph.start ?? '',
    nodes: Array.isArray(graph.nodes) ? graph.nodes : [],
  })
  const g = reactFlowToGraph(nodes, edges, graph.start ?? '')
  return JSON.stringify({ start: g.start ?? '', nodes: g.nodes ?? [] })
}

// needsLayout reports whether any node lacks a persisted position.
function needsLayout(graph: FlowGraph): boolean {
  return graph.nodes.some((n) => n.x === undefined || n.y === undefined)
}

// autoLayout assigns a layered grid position to every node VERTICALLY: the BFS
// depth from start flows top→bottom (y), and siblings at the same depth spread
// left→right (x). Cyclic graphs are bounded by a visited set so this always
// terminates.
export function autoLayout(graph: FlowGraph): Record<string, { x: number; y: number }> {
  const byId = new Map(graph.nodes.map((n) => [n.id, n]))
  const depth = new Map<string, number>()
  const queue: Array<{ id: string; d: number }> = []
  if (graph.start && byId.has(graph.start)) queue.push({ id: graph.start, d: 0 })

  while (queue.length) {
    const { id, d } = queue.shift()!
    if (depth.has(id)) {
      depth.set(id, Math.max(depth.get(id)!, d))
      continue
    }
    depth.set(id, d)
    for (const t of successors(byId.get(id))) {
      if (t && byId.has(t)) queue.push({ id: t, d: d + 1 })
    }
  }

  // Unreachable nodes get appended at increasing depths so they're still visible.
  let extra = (depth.size ? Math.max(...depth.values()) : -1) + 1
  for (const n of graph.nodes) if (!depth.has(n.id)) depth.set(n.id, extra++)

  // Vertical layout: depth = row (down the y-axis), sibling order = column
  // (spread across the x-axis) so the flow reads top→bottom.
  const colByDepth = new Map<number, number>()
  const pos: Record<string, { x: number; y: number }> = {}
  for (const n of graph.nodes) {
    const d = depth.get(n.id) ?? 0
    const col = colByDepth.get(d) ?? 0
    colByDepth.set(d, col + 1)
    pos[n.id] = { x: col * COL_W, y: d * ROW_H }
  }
  return pos
}

// successors lists the node ids a node routes to, across all node types.
function successors(n: FlowNode | undefined): string[] {
  if (!n) return []
  switch (n.type) {
    case 'agent':
    case 'delay':
    case 'transform':
    case 'await-input':
    case 'subflow':
    case 'start':
    case 'spawn':
    case 'join':
    case 'coordinator':
      return [n.next ?? '']
    case 'end':
      return []
    case 'branch':
      return (n.branches ?? []).map((b) => b.next)
    case 'parallel':
      return [...(n.parallel ?? []), n.joinNext ?? '']
    case 'loop':
      return [n.body ?? '', n.loopNext ?? '']
    default:
      return []
  }
}

function round(v: number): number {
  return Math.round(v)
}

// ensureStartNode upgrades a graph to the required start-node model: when it lacks
// a start node it prepends one (Next = the old entry) and repoints start to it.
// Mirrors the backend's MigrateAddStart so templates/previews render + instantiate
// validly. Idempotent.
export function ensureStartNode(graph: FlowGraph): FlowGraph {
  if (graph.nodes.some((n) => n.type === 'start')) return graph
  const id = graph.nodes.some((n) => n.id === 'start') ? `start_${graph.nodes.length}` : 'start'
  const startNode: FlowNode = { id, type: 'start', title: 'Başlangıç', next: graph.start || '' }
  return { ...graph, start: id, nodes: [startNode, ...graph.nodes] }
}

// nextNodeId returns the smallest unused "n<i>" id for a new node.
export function nextNodeId(nodes: FlowNode[]): string {
  let i = 1
  while (nodes.some((n) => n.id === `n${i}`)) i++
  return `n${i}`
}

// blankNode builds a default node of the given type.
export function blankNode(id: string, type: FlowNodeType, defaultAgentId = ''): FlowNode {
  const node: FlowNode = { id, type, title: '' }
  if (type === 'agent') {
    node.agentId = defaultAgentId
    node.prompt = '{{input}}'
    node.next = ''
  } else if (type === 'branch') {
    node.branches = [{ contains: '', next: '' }]
    node.matchMode = 'contains'
  } else if (type === 'delay') {
    node.delayMs = 1000
    node.next = ''
  } else if (type === 'transform') {
    node.template = '{{last}}'
    node.next = ''
  } else if (type === 'loop') {
    node.body = ''
    node.loopNext = ''
    node.maxIters = 3
    node.until = ''
    node.untilMode = 'contains'
  } else if (type === 'await-input') {
    node.next = ''
    node.title = 'Girdi bekle'
  } else if (type === 'subflow') {
    node.flowRef = ''
    node.template = '{{last}}'
    node.next = ''
    node.title = 'Alt-akış'
  } else if (type === 'start') {
    node.next = ''
    node.title = 'Başlangıç'
  } else if (type === 'end') {
    node.title = 'Bitiş'
  } else if (type === 'spawn') {
    node.spawnFlows = []
    node.template = '{{last}}'
    node.next = ''
    node.title = 'Spawn'
  } else if (type === 'join') {
    node.spawnRef = ''
    node.next = ''
    node.title = 'Join'
  } else if (type === 'coordinator') {
    node.agentId = defaultAgentId
    node.prompt = '{{last}}'
    node.next = ''
    node.title = 'Koordinatör'
  } else {
    node.parallel = []
    node.joinNext = ''
  }
  return node
}
