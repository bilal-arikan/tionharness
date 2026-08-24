import { describe, expect, it } from 'vitest'
import { parseRef, refToString, type ViewRef } from '@/types'
import {
  buildGraph,
  isDrillable,
  nextExpandedSet,
  ROOT_KEY,
  type GraphInputs,
} from './explorerModel'

const sessions: ViewRef = { kind: 'category', id: 'sessions' }
const board: ViewRef = { kind: 'board', id: 'board' }
const s1: ViewRef = { kind: 'session', id: 'S1' }
const s2: ViewRef = { kind: 'session', id: 'S2' }

// baseInputs wires a small tree: workspace → [category:sessions, board]; the
// sessions category → [S1, S2]. Callers flip `expanded`/`selectedKey`/`search`.
function baseInputs(over: Partial<GraphInputs> = {}): GraphInputs {
  const childrenByKey: Record<string, ViewRef[]> = {
    [ROOT_KEY]: [sessions, board],
    [refToString(sessions)]: [s1, s2],
  }
  const refByKey: Record<string, ViewRef> = {}
  const labelByKey: Record<string, string> = { [ROOT_KEY]: 'Workspace' }
  for (const r of [sessions, board, s1, s2]) {
    refByKey[refToString(r)] = r
    labelByKey[refToString(r)] = refToString(r)
  }
  return {
    refByKey,
    labelByKey,
    childrenByKey,
    expanded: new Set(),
    loading: new Set(),
    selectedKey: ROOT_KEY,
    search: '',
    ...over,
  }
}

describe('buildGraph', () => {
  it('shows only the root until it is expanded', () => {
    const { nodes } = buildGraph(baseInputs())
    expect(nodes.map((n) => n.id)).toEqual([ROOT_KEY])
  })

  it('adds one layer per expanded node', () => {
    const { nodes, edges } = buildGraph(
      baseInputs({ expanded: new Set([ROOT_KEY, refToString(sessions)]) }),
    )
    const ids = nodes.map((n) => n.id).sort()
    expect(ids).toEqual(
      [ROOT_KEY, 'board:board', 'category:sessions', 'session:S1', 'session:S2'].sort(),
    )
    // An edge is drawn parent → child for every placed member.
    expect(edges.some((e) => e.source === ROOT_KEY && e.target === 'category:sessions')).toBe(true)
    expect(edges.some((e) => e.source === 'category:sessions' && e.target === 'session:S1')).toBe(
      true,
    )
  })

  it('lays out one column per depth', () => {
    const { nodes } = buildGraph(
      baseInputs({ expanded: new Set([ROOT_KEY, refToString(sessions)]) }),
    )
    const x = (id: string) => nodes.find((n) => n.id === id)!.position.x
    expect(x(ROOT_KEY)).toBeLessThan(x('category:sessions'))
    expect(x('category:sessions')).toBeLessThan(x('session:S1'))
  })

  it('places a node once even when a child points back to it (cycle break)', () => {
    // S1 lists the sessions category as a child → a cycle. It must not loop.
    const inputs = baseInputs({
      expanded: new Set([ROOT_KEY, refToString(sessions), refToString(s1)]),
    })
    inputs.childrenByKey[refToString(s1)] = [sessions]
    const { nodes } = buildGraph(inputs)
    expect(nodes.filter((n) => n.id === 'category:sessions')).toHaveLength(1)
  })

  it('dims nodes off the degree-of-interest focus when a non-root node is selected', () => {
    const { nodes } = buildGraph(
      baseInputs({
        expanded: new Set([ROOT_KEY, refToString(sessions)]),
        selectedKey: refToString(sessions),
      }),
    )
    const dim = (id: string) => nodes.find((n) => n.id === id)!.data.dimmed
    // Focus = selected + ancestors (root) + direct children (S1, S2).
    expect(dim('category:sessions')).toBe(false)
    expect(dim(ROOT_KEY)).toBe(false)
    expect(dim('session:S1')).toBe(false)
    // The board is a sibling off the focus path → dimmed.
    expect(dim('board:board')).toBe(true)
  })

  it('does not dim anything when the root is selected', () => {
    const { nodes } = buildGraph(
      baseInputs({ expanded: new Set([ROOT_KEY]), selectedKey: ROOT_KEY }),
    )
    expect(nodes.every((n) => !n.data.dimmed)).toBe(true)
  })

  it('search dims every node whose label does not match', () => {
    const { nodes } = buildGraph(
      baseInputs({ expanded: new Set([ROOT_KEY, refToString(sessions)]), search: 'S1' }),
    )
    const dim = (id: string) => nodes.find((n) => n.id === id)!.data.dimmed
    expect(dim('session:S1')).toBe(false)
    expect(dim('session:S2')).toBe(true)
    expect(dim(ROOT_KEY)).toBe(true)
  })
})

describe('isDrillable', () => {
  it('treats a single board card (board with sub) as a leaf', () => {
    expect(isDrillable(board)).toBe(true)
    expect(isDrillable({ kind: 'board', id: 'board', sub: 'T1' })).toBe(false)
    expect(isDrillable({ kind: 'budget', id: 'budget' })).toBe(false)
    expect(isDrillable(sessions)).toBe(true)
  })

  it('treats the TSK66 leaves as leaves', () => {
    for (const ref of [
      { kind: 'artifact', id: 'ART1' },
      { kind: 'automation', id: 'AUT1' },
      { kind: 'skill', id: 'tionharness-build' },
      { kind: 'insight', id: 'FND1' },
      { kind: 'logs', id: 'logs' },
    ] as ViewRef[]) {
      expect(isDrillable(ref)).toBe(false)
    }
    // The extension BUCKETS themselves are categories → drillable.
    expect(isDrillable({ kind: 'category', id: 'artifacts' })).toBe(true)
    expect(isDrillable({ kind: 'category', id: 'skills' })).toBe(true)
  })
})

describe('nextExpandedSet (single-expand accordion)', () => {
  // Tree: root → [sessions, board]; sessions → [S1, S2]. Parents are recorded as
  // the hook would after fetching children.
  const parentByKey: Record<string, string> = {
    'category:sessions': ROOT_KEY,
    'board:board': ROOT_KEY,
    'session:S1': 'category:sessions',
    'session:S2': 'category:sessions',
  }

  it('expanding a node collapses every other open sibling', () => {
    const expanded = new Set([ROOT_KEY, 'category:sessions'])
    const next = nextExpandedSet(expanded, 'board:board', parentByKey)
    expect(next.has('board:board')).toBe(true)
    expect(next.has('category:sessions')).toBe(false) // same parent → closed
    expect(next.has(ROOT_KEY)).toBe(true) // the root is not a sibling
  })

  it("keeps nodes in OTHER branches open (only the parent's children collapse)", () => {
    // S1 and S2 are siblings under sessions; board is a different branch.
    const expanded = new Set([ROOT_KEY, 'category:sessions', 'session:S1', 'board:board'])
    const next = nextExpandedSet(expanded, 'session:S2', parentByKey)
    expect(next.has('session:S2')).toBe(true)
    expect(next.has('session:S1')).toBe(false) // sibling under the same parent
    expect(next.has('category:sessions')).toBe(true) // ancestor stays
    expect(next.has('board:board')).toBe(true) // unrelated branch stays
    expect(next.has(ROOT_KEY)).toBe(true)
  })

  it('collapsing a node touches nothing else', () => {
    const expanded = new Set([ROOT_KEY, 'category:sessions', 'board:board'])
    const next = nextExpandedSet(expanded, 'category:sessions', parentByKey)
    expect(next.has('category:sessions')).toBe(false)
    expect(next.has('board:board')).toBe(true)
    expect(next.has(ROOT_KEY)).toBe(true)
  })

  it('expanding with no open siblings only adds', () => {
    const expanded = new Set([ROOT_KEY])
    const next = nextExpandedSet(expanded, 'category:sessions', parentByKey)
    expect([...next].sort()).toEqual([ROOT_KEY, 'category:sessions'].sort())
  })
})

describe('parseRef', () => {
  it('round-trips a category id that contains its own colon', () => {
    const ref: ViewRef = { kind: 'category', id: 'col:in_progress' }
    expect(parseRef(refToString(ref))).toEqual(ref)
  })

  it('round-trips a board card with a sub selector', () => {
    const ref: ViewRef = { kind: 'board', id: 'board', sub: 'T1' }
    expect(parseRef(refToString(ref))).toEqual(ref)
  })

  it('rejects a string that does not name a known kind', () => {
    expect(parseRef('galaxy:x')).toBeNull()
    expect(parseRef('nocolon')).toBeNull()
  })
})
