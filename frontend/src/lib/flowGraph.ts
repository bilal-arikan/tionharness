// Adapter between SwarmGo's orchestration graph (FlowGraph: start + nodes with
// next/branches/parallel/joinNext) and React Flow's nodes+edges model. The
// FlowNode itself is carried as RFNode.data so custom node components and the
// inspector edit it directly; edges are derived from the routing fields.
import type { Edge, Node as RFNode } from '@xyflow/react'
import type { FlowGraph, FlowNode, FlowNodeType } from '../types'

export type FlowRFNode = RFNode<{ node: FlowNode; isStart: boolean; status?: NodeStatus }>
export type NodeStatus = 'running' | 'done' | 'error'

// Layout grid spacing for auto-placed nodes.
const COL_W = 280
const ROW_H = 140

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
    data: { node: n, isStart: n.id === graph.start },
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
        add(n.id, n.next ?? '')
        break
      case 'branch':
      case 'switch':
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
        base.next = outgoing[0]?.target ?? ''
        break
      case 'branch':
      case 'switch': {
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
    }
    return base
  })
  return { start, nodes: out }
}

// needsLayout reports whether any node lacks a persisted position.
function needsLayout(graph: FlowGraph): boolean {
  return graph.nodes.some((n) => n.x === undefined || n.y === undefined)
}

// autoLayout assigns a layered grid position to every node: column = BFS depth
// from start, row = order within that depth. Cyclic graphs are bounded by a
// visited set so this always terminates.
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

  const rowByCol = new Map<number, number>()
  const pos: Record<string, { x: number; y: number }> = {}
  for (const n of graph.nodes) {
    const col = depth.get(n.id) ?? 0
    const row = rowByCol.get(col) ?? 0
    rowByCol.set(col, row + 1)
    pos[n.id] = { x: col * COL_W, y: row * ROW_H }
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
      return [n.next ?? '']
    case 'branch':
    case 'switch':
      return (n.branches ?? []).map((b) => b.next)
    case 'parallel':
      return [...(n.parallel ?? []), n.joinNext ?? '']
    default:
      return []
  }
}

function round(v: number): number {
  return Math.round(v)
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
  } else if (type === 'branch' || type === 'switch') {
    node.branches = [{ contains: '', next: '' }]
  } else if (type === 'delay') {
    node.delayMs = 1000
    node.next = ''
  } else if (type === 'transform') {
    node.template = '{{last}}'
    node.next = ''
  } else {
    node.parallel = []
    node.joinNext = ''
  }
  return node
}
