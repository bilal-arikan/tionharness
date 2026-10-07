import { describe, expect, it } from 'vitest'
import type { ViewGraphResult, ViewRef } from '@/types'
import {
  applyExplorerFilter,
  childCounts,
  countActiveExplorerFacets,
  defaultExplorerFilter,
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

describe('collapsed nodes', () => {
  const layer = augmentLive(graph)

  it('keeps a folded node but drops what only it reaches; counts its children', () => {
    const out = applyExplorerFilter(
      layer.graph,
      emptyExplorerFilter(),
      layer.liveState,
      ROOT,
      new Set(['category:agents']),
    )
    const k = keys(out)
    expect(k).toContain('category:agents')
    expect(k).not.toContain('agent:AG1')
    expect(k).not.toContain('agent:AG2')
    // Sessions still arrive through the sessions bucket.
    expect(k).toContain('session:SES1')
    const foldedSession = applyExplorerFilter(
      layer.graph,
      emptyExplorerFilter(),
      layer.liveState,
      ROOT,
      new Set(['session:SES1']),
    )
    expect(keys(foldedSession)).toContain('session:SES1')
    expect(keys(foldedSession)).not.toContain('session:W1')
    expect(keys(foldedSession)).not.toContain('agent:AG1#live:SES1')
    const counts = childCounts(layer.graph)
    expect(counts.get('category:agents')).toBe(2)
    expect(counts.get('session:SES1')).toBe(2) // worker + live avatar
    expect(counts.get('session:W1')).toBeUndefined()
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
    expect(parseExplorerFilter(null)).toEqual(defaultExplorerFilter())
    expect(parseExplorerFilter('{nope')).toEqual(defaultExplorerFilter())
    expect(
      parseExplorerFilter(JSON.stringify({ kinds: 'chat', liveOnly: 'yes', tags: [1, 'a'] })),
    ).toEqual({
      ...defaultExplorerFilter(),
      tags: ['a'],
    })
    expect(countActiveExplorerFacets(f)).toBe(3)
    expect(countActiveExplorerFacets(emptyExplorerFilter())).toBe(0)
    expect(toggleValue(['a'], 'a')).toEqual([])
    expect(toggleValue(['a'], 'b')).toEqual(['a', 'b'])
  })
})

describe('attention facet', () => {
  const card = r('board', 'board', 'TSK1')
  const board = r('board', 'board')
  const withAttention: ViewGraphResult = {
    nodes: [...graph.nodes, { label: 'Pano', ref: board }, { label: 'TSK1 broken', ref: card }],
    edges: [...graph.edges, { source: root, target: board }, { source: board, target: card }],
    attention: {
      'session:SES1': { level: 'danger', reasons: ['stuck'] },
      'board:board#TSK1': { level: 'danger', reasons: ['failed-card'] },
    },
  }

  it('keeps only the sessions and cards that need a look, and the structure around them', () => {
    const live = augmentLive(withAttention)
    const out = applyExplorerFilter(
      live.graph,
      { ...emptyExplorerFilter(), attention: ['stuck'] },
      live.liveState,
      ROOT,
    )
    const keys = out.nodes.map(
      (h) => `${h.ref.kind}:${h.ref.id}${h.ref.sub ? '#' + h.ref.sub : ''}`,
    )
    expect(keys).toContain('session:SES1')
    expect(keys).not.toContain('session:SES2')
    expect(keys).not.toContain('board:board#TSK1')
    expect(keys).toContain('category:sessions')
    expect(keys).toContain('board:board')
  })

  it('round-trips the facet and drops unknown values', () => {
    const parsed = parseExplorerFilter(
      serializeExplorerFilter({
        ...emptyExplorerFilter(),
        attention: ['failed', 'bogus'] as string[],
      }),
    )
    expect(parsed.attention).toEqual(['failed'])
    expect(countActiveExplorerFacets(parsed)).toBe(1)
  })
})

describe('time window', () => {
  const NOW = 1_800_000_000
  const nowMs = NOW * 1000
  const timed: ViewGraphResult = {
    ...graph,
    live: [],
    times: {
      // SES2 was edited 2h ago, W1 only read by an agent 10 min ago, SES1 is a
      // day old; AG1/AG2 were created long ago.
      'session:SES1': { created: NOW - 90_000, updated: NOW - 86_500 },
      'session:SES2': { created: NOW - 90_000, updated: NOW - 7_200 },
      'session:W1': { created: NOW - 90_000, read: NOW - 600, readBy: 'AG2' },
      'agent:AG1': { created: NOW - 900_000 },
      'agent:AG2': { created: NOW - 900_000 },
    },
  }
  const cut = (window: 0 | 3600 | 21600 | 86400 | 259200, g = timed) => {
    const live = augmentLive(g)
    return keys(
      applyExplorerFilter(
        live.graph,
        { ...emptyExplorerFilter(), window },
        live.liveState,
        ROOT,
        undefined,
        nowMs,
      ),
    )
  }

  it('keeps nodes whose newest stamp (created, edited or read) is inside the window', () => {
    const hour = cut(3600)
    // W1 counts through its agent read; its coordinator SES1 stays as its parent.
    expect(hour).toContain('session:W1')
    expect(hour).toContain('session:SES1')
    expect(hour).not.toContain('session:SES2')
    expect(hour).not.toContain('agent:AG2')
    // Untimed nodes with nothing recent below them go too.
    expect(hour).not.toContain('tools:tools')
    expect(hour).toContain(ROOT)

    const sixHours = cut(21600)
    expect(sixHours).toContain('session:SES2')
    expect(sixHours).toContain('category:sessions')
    // An agent above a recent session is on its path, so it stays.
    expect(sixHours).toContain('agent:AG2')
  })

  it('is the identity at "all time" and keeps live sessions whatever their stamps', () => {
    expect(cut(0)).toHaveLength(augmentLive(timed).graph.nodes.length)
    const withLive = cut(3600, { ...timed, live: graph.live })
    expect(withLive).toContain('session:SES1')
    expect(withLive).toContain('agent:AG1#live:SES1')
  })

  it('does not cut a payload that carries no stamps', () => {
    const live = augmentLive(graph)
    const out = applyExplorerFilter(
      live.graph,
      { ...emptyExplorerFilter(), window: 3600 },
      live.liveState,
      ROOT,
      undefined,
      nowMs,
    )
    expect(out.nodes).toHaveLength(live.graph.nodes.length)
  })

  it('persists the window, defaults to a day and is not counted as a facet', () => {
    const f = { ...emptyExplorerFilter(), window: 3600 as const }
    expect(parseExplorerFilter(serializeExplorerFilter(f))).toEqual(f)
    expect(parseExplorerFilter(JSON.stringify({ window: 42 })).window).toBe(86400)
    expect(parseExplorerFilter(JSON.stringify({ kinds: [] })).window).toBe(86400)
    expect(countActiveExplorerFacets(f)).toBe(0)
  })
})
