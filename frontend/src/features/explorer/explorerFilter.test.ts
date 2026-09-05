import { describe, expect, it } from 'vitest'
import type { ViewGraphResult, ViewRef } from '@/types'
import {
  applyExplorerFilter,
  countActiveExplorerFacets,
  emptyExplorerFilter,
  explorerFacets,
  parseExplorerFilter,
  serializeExplorerFilter,
  toggleValue,
} from './explorerFilter'
import { augmentLive } from './explorerLive'

const ROOT = 'workspace:workspace'
const r = (kind: ViewRef['kind'], id: string, sub?: string): ViewRef => ({ kind, id, sub })
const root = r('workspace', 'workspace')
const sessions = r('category', 'sessions')
const agents = r('category', 'agents')
const tools = r('tools', 'tools')
const ag1 = r('agent', 'AG1')
const ag2 = r('agent', 'AG2')
const s1 = r('session', 'SES1')
const s2 = r('session', 'SES2')
const w1 = r('session', 'W1')
const mcp = r('tools', 'tools', 'mcp:M1')

const graph: ViewGraphResult = {
  nodes: [
    { label: 'workspace', ref: root },
    { label: 'Oturumlar', ref: sessions },
    { label: 'Ajanlar', ref: agents },
    { label: 'Araçlar', ref: tools },
    { label: 'agent:AG1 builder', ref: ag1 },
    { label: 'agent:AG2 researcher', ref: ag2 },
    { label: 'session:SES1 a', ref: s1 },
    { label: 'session:SES2 b', ref: s2 },
    { label: 'session:W1 w', ref: w1 },
    { label: 'MCP linear', ref: mcp },
  ],
  edges: [
    { source: root, target: sessions },
    { source: root, target: agents },
    { source: root, target: tools },
    { source: agents, target: ag1 },
    { source: agents, target: ag2 },
    { source: sessions, target: s1 },
    { source: sessions, target: s2 },
    { source: ag1, target: s1 },
    { source: ag2, target: s2 },
    { source: s1, target: w1 }, // worker under coordinator, not in the sessions bucket
    { source: tools, target: mcp },
  ],
  live: [{ session: s1, state: 'running', agent: { id: 'AG1', name: 'builder' } }],
  meta: {
    'session:SES1': { kind: 'chat', agentId: 'AG1', tags: ['x'] },
    'session:SES2': { kind: 'task', agentId: 'AG2', tags: ['y'] },
    'session:W1': { kind: 'worker', agentId: 'AG1' },
  },
}

const keys = (g: ViewGraphResult) =>
  g.nodes.map((h) => `${h.ref.kind}:${h.ref.id}${h.ref.sub ? '#' + h.ref.sub : ''}`)

describe('applyExplorerFilter', () => {
  const layer = augmentLive(graph)

  it('is the identity for the empty filter', () => {
    const out = applyExplorerFilter(layer.graph, emptyExplorerFilter(), layer.liveState, ROOT)
    expect(out.nodes).toHaveLength(layer.graph.nodes.length)
    expect(out.edges).toHaveLength(layer.graph.edges.length)
  })

  it('hides a bucket with everything only it reaches, keeping what another path still reaches', () => {
    const out = applyExplorerFilter(
      layer.graph,
      { ...emptyExplorerFilter(), hiddenBuckets: ['category:sessions'] },
      layer.liveState,
      ROOT,
    )
    const k = keys(out)
    expect(k).not.toContain('category:sessions')
    // SES1/SES2 survive through their agents; the worker through SES1.
    expect(k).toContain('session:SES1')
    expect(k).toContain('session:W1')
    expect(k).toContain('agent:AG1#live:SES1')
    const hidTools = applyExplorerFilter(
      layer.graph,
      { ...emptyExplorerFilter(), hiddenBuckets: ['tools:tools'] },
      layer.liveState,
      ROOT,
    )
    expect(keys(hidTools)).not.toContain('tools:tools#mcp:M1')
  })

  it('live-only keeps executing sessions, their avatars and the structure around them', () => {
    const out = applyExplorerFilter(
      layer.graph,
      { ...emptyExplorerFilter(), liveOnly: true },
      layer.liveState,
      ROOT,
    )
    const k = keys(out)
    expect(k).toContain('session:SES1')
    expect(k).toContain('agent:AG1#live:SES1')
    expect(k).not.toContain('session:SES2')
    expect(k).not.toContain('session:W1')
    expect(k).toContain('category:sessions')
  })

  it('kind, agent and tag facets narrow sessions and drop their orphaned subtrees', () => {
    const byKind = applyExplorerFilter(
      layer.graph,
      { ...emptyExplorerFilter(), kinds: ['task'] },
      layer.liveState,
      ROOT,
    )
    expect(keys(byKind)).toContain('session:SES2')
    expect(keys(byKind)).not.toContain('session:SES1')
    expect(keys(byKind)).not.toContain('session:W1')
    const byAgent = applyExplorerFilter(
      layer.graph,
      { ...emptyExplorerFilter(), agentIds: ['AG1'] },
      layer.liveState,
      ROOT,
    )
    expect(keys(byAgent)).toEqual(expect.arrayContaining(['session:SES1', 'session:W1']))
    expect(keys(byAgent)).not.toContain('session:SES2')
    const byTag = applyExplorerFilter(
      layer.graph,
      { ...emptyExplorerFilter(), tags: ['y'] },
      layer.liveState,
      ROOT,
    )
    expect(keys(byTag)).toContain('session:SES2')
    expect(keys(byTag)).not.toContain('session:SES1')
    // Facets AND together.
    const both = applyExplorerFilter(
      layer.graph,
      { ...emptyExplorerFilter(), tags: ['y'], agentIds: ['AG1'] },
      layer.liveState,
      ROOT,
    )
    expect(keys(both).filter((k) => k.startsWith('session:'))).toEqual([])
  })
})

describe('explorerFacets', () => {
  it('reads the vocabularies off the graph', () => {
    expect(explorerFacets(graph)).toEqual({
      kinds: ['chat', 'task', 'worker'],
      agents: [
        { id: 'AG1', label: 'builder' },
        { id: 'AG2', label: 'researcher' },
      ],
      tags: ['x', 'y'],
    })
  })
})

describe('filter persistence helpers', () => {
  it('round-trips, tolerates garbage and counts facets', () => {
    const f = {
      ...emptyExplorerFilter(),
      liveOnly: true,
      kinds: ['chat'],
      hiddenBuckets: ['tools:tools'],
    }
    expect(parseExplorerFilter(serializeExplorerFilter(f))).toEqual(f)
    expect(parseExplorerFilter(null)).toEqual(emptyExplorerFilter())
    expect(parseExplorerFilter('{nope')).toEqual(emptyExplorerFilter())
    expect(
      parseExplorerFilter(JSON.stringify({ kinds: 'chat', liveOnly: 'yes', tags: [1, 'a'] })),
    ).toEqual({
      ...emptyExplorerFilter(),
      tags: ['a'],
    })
    expect(countActiveExplorerFacets(f)).toBe(3)
    expect(countActiveExplorerFacets(emptyExplorerFilter())).toBe(0)
    expect(toggleValue(['a'], 'a')).toEqual([])
    expect(toggleValue(['a'], 'b')).toEqual(['a', 'b'])
  })
})
