import { describe, expect, it } from 'vitest'
import type { FlowGraph } from '@/types'
import {
  applyPositions,
  autoLayout,
  canonicalKey,
  defaultNode,
  freshEdgeId,
  freshNodeId,
  lint,
  needsLayout,
  parseGraph,
  shapeOf,
  toReactFlow,
} from './flowGraph'

const base: FlowGraph = {
  version: 2,
  nodes: [
    { id: 'input', type: 'input' },
    { id: 'respond', type: 'llm', prompt: '{{input}}' },
    { id: 'critic', type: 'llm', context: 'fresh', prompt: 'Review {{node.respond}}' },
    { id: 'check', type: 'route' },
    { id: 'output', type: 'output' },
  ],
  edges: [
    { id: 'e1', from: 'input', to: 'respond' },
    { id: 'e2', from: 'respond', to: 'critic' },
    { id: 'e3', from: 'critic', to: 'check' },
    { id: 'e4', from: 'check', to: 'output', when: 'APPROVE' },
    { id: 'e5', from: 'check', to: 'respond' },
  ],
}

describe('flowGraph', () => {
  it('parses the backend JSON and tolerates garbage', () => {
    expect(parseGraph(JSON.stringify(base)).nodes).toHaveLength(5)
    expect(parseGraph('nope').nodes).toEqual([])
  })

  it('lays out by depth with the output at the bottom and loop targets kept above', () => {
    const g = autoLayout(base)
    const y = (id: string) => g.nodes.find((n) => n.id === id)!.y!
    expect(y('input')).toBeLessThan(y('respond'))
    expect(y('respond')).toBeLessThan(y('critic'))
    expect(y('critic')).toBeLessThan(y('check'))
    expect(y('check')).toBeLessThan(y('output'))
    expect(needsLayout(base)).toBe(true)
    expect(needsLayout(g)).toBe(false)
  })

  it('ignores positions in the dirty key but not in the layout', () => {
    const moved = applyPositions(
      toReactFlowGraph(base),
      toReactFlow(base, new Map(), new Map()).nodes.map((n) => ({
        ...n,
        position: { x: 10, y: 20 },
      })),
    )
    expect(canonicalKey(moved)).toBe(canonicalKey(base))
    expect(moved.nodes[0].x).toBe(10)
  })

  it('lints the structural invariants', () => {
    expect(lint(base)).toEqual([])
    const broken: FlowGraph = { ...base, edges: base.edges.filter((e) => e.id !== 'e1') }
    expect(lint(broken).map((p) => p.key)).toContain('lint.oneOutgoing')
    const noOut: FlowGraph = {
      ...base,
      nodes: base.nodes.filter((n) => n.type !== 'output'),
      edges: base.edges.filter((e) => e.to !== 'output'),
    }
    expect(lint(noOut).map((p) => p.key)).toContain('lint.oneOutput')
  })

  it('renders the route arms in the shape line and on the canvas node', () => {
    expect(shapeOf(base)).toContain('check(route: APPROVE→output, *→respond)')
    const rf = toReactFlow(base, new Map([['critic', 'running']]), new Map())
    const check = rf.nodes.find((n) => n.id === 'check')!
    expect(check.data.arms).toEqual([
      { when: 'APPROVE', to: 'output' },
      { when: '', to: 'respond' },
    ])
    expect(rf.nodes.find((n) => n.id === 'critic')!.data.status).toBe('running')
    expect(rf.edges.find((e) => e.id === 'e4')!.data?.isRoute).toBe(true)
    expect(rf.edges.find((e) => e.id === 'e1')!.data?.isRoute).toBe(false)
  })

  it('allocates fresh ids and typed defaults', () => {
    expect(freshNodeId(base, 'critic')).toBe('critic_2')
    expect(freshEdgeId(base, 'check', 'respond')).toBe('e_check_respond')
    expect(defaultNode('route', 'r', 'R').maxVisits).toBe(3)
    expect(defaultNode('llm', 'l', 'L').context).toBe('thread')
  })
})

function toReactFlowGraph(g: FlowGraph): FlowGraph {
  return { ...g, nodes: g.nodes.map((n) => ({ ...n })) }
}
