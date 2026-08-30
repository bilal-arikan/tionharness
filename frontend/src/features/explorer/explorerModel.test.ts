import { describe, expect, it } from 'vitest'
import { parseRef, refToString, type ViewHandle, type ViewNeighborhoodResult } from '@/types'
import { buildFocusGraph, VISIBLE_RELATIONS_PER_SIDE } from './explorerModel'

const handle = (kind: 'session' | 'agent' | 'category', id: string): ViewHandle => ({
  label: `${kind}-${id}`,
  ref: { kind, id },
})

function neighborhood(over: Partial<ViewNeighborhoodResult> = {}): ViewNeighborhoodResult {
  return {
    focus: handle('session', 'S1'),
    parents: [handle('category', 'sessions')],
    children: [handle('session', 'S2')],
    hiddenParentCount: 0,
    hiddenChildCount: 0,
    ...over,
  }
}

describe('buildFocusGraph', () => {
  it('renders exactly direct parents, focus and direct children', () => {
    const { nodes, edges } = buildFocusGraph({
      neighborhood: neighborhood(),
      selectedKey: 'session:S2',
      search: '',
      loading: false,
    })
    expect(nodes.map((node) => node.id).sort()).toEqual(
      ['category:sessions', 'session:S1', 'session:S2'].sort(),
    )
    expect(edges.map((edge) => `${edge.source}->${edge.target}`)).toEqual([
      'category:sessions->session:S1',
      'session:S1->session:S2',
    ])
    expect(nodes.find((node) => node.id === 'session:S2')?.data.selected).toBe(true)
  })

  it('deduplicates multi-parent refs and sorts nodes deterministically by ref', () => {
    const input = neighborhood({
      parents: [handle('agent', 'Z'), handle('agent', 'A'), handle('agent', 'A')],
      children: [],
    })
    const result = buildFocusGraph({
      neighborhood: input,
      selectedKey: null,
      search: '',
      loading: false,
    })
    expect(result.nodes.map((node) => node.id)).toEqual(['agent:A', 'agent:Z', 'session:S1'])
    expect(result.edges).toHaveLength(2)
  })

  it('places a ref present as parent and child once while retaining both directed edges', () => {
    const shared = handle('session', 'S2')
    const { nodes, edges } = buildFocusGraph({
      neighborhood: neighborhood({ parents: [shared], children: [shared] }),
      selectedKey: null,
      search: '',
      loading: false,
    })
    expect(nodes.filter((node) => node.id === 'session:S2')).toHaveLength(1)
    expect(edges.map((edge) => `${edge.source}->${edge.target}`).sort()).toEqual(
      ['session:S1->session:S2', 'session:S2->session:S1'].sort(),
    )
    expect(edges.every((edge) => edge.style?.strokeDasharray === '7 5')).toBe(true)
    expect(edges.every((edge) => edge.label === 'iki yönlü')).toBe(true)
  })

  it('deduplicates a self-loop edge and never duplicates focus node', () => {
    const focus = handle('session', 'SELF')
    const { nodes, edges } = buildFocusGraph({
      neighborhood: neighborhood({ focus, parents: [focus], children: [focus] }),
      selectedKey: 'session:SELF',
      search: '',
      loading: true,
    })
    expect(nodes).toHaveLength(1)
    expect(edges).toHaveLength(1)
    expect(edges[0]).toMatchObject({ source: 'session:SELF', target: 'session:SELF' })
    expect(edges[0].label).toBe('cycle')
    expect(edges[0].ariaLabel).toContain('cycle')
    expect(nodes[0].data.loading).toBe(true)
  })

  it('caps each canvas side deterministically and retains every remainder in overflow', () => {
    const parents = Array.from({ length: 9 }, (_, index) => handle('agent', `P${index}`)).reverse()
    const children = Array.from({ length: 10 }, (_, index) =>
      handle('session', `C${index}`),
    ).reverse()

    const result = buildFocusGraph({
      neighborhood: neighborhood({ parents, children }),
      selectedKey: null,
      search: '',
      loading: false,
    })

    expect(
      result.nodes.filter((node) => node.data.depth === 0 && !node.data.overflow),
    ).toHaveLength(VISIBLE_RELATIONS_PER_SIDE)
    expect(
      result.nodes.filter((node) => node.data.depth === 2 && !node.data.overflow),
    ).toHaveLength(VISIBLE_RELATIONS_PER_SIDE)
    expect(result.overflow.parents.map((item) => item.ref.id)).toEqual(['P6', 'P7', 'P8'])
    expect(result.overflow.children.map((item) => item.ref.id)).toEqual(['C6', 'C7', 'C8', 'C9'])
    expect(result.nodes.find((node) => node.id === '__overflow:parents')?.data.label).toBe(
      '+3 üst bağlantı',
    )
    expect(result.nodes.find((node) => node.id === '__overflow:children')?.data.label).toBe(
      '+4 alt bağlantı',
    )
    expect(result.edges).toHaveLength(VISIBLE_RELATIONS_PER_SIDE * 2)
  })

  it('keeps an empty neighborhood as a real focus node', () => {
    const { nodes, edges } = buildFocusGraph({
      neighborhood: neighborhood({ parents: [], children: [] }),
      selectedKey: 'session:S1',
      search: '',
      loading: false,
    })
    expect(nodes.map((node) => node.id)).toEqual(['session:S1'])
    expect(edges).toEqual([])
    expect(nodes[0].data.childCount).toBe(0)
  })

  it('dims only non-matching visible nodes during search', () => {
    const { nodes } = buildFocusGraph({
      neighborhood: neighborhood(),
      selectedKey: null,
      search: 'session-S2',
      loading: false,
    })
    expect(nodes.find((node) => node.id === 'session:S2')?.data.dimmed).toBe(false)
    expect(nodes.find((node) => node.id === 'session:S1')?.data.dimmed).toBe(true)
  })
})

describe('parseRef', () => {
  it('round-trips refs containing colon and sub selectors', () => {
    for (const ref of [
      { kind: 'category' as const, id: 'col:in_progress' },
      { kind: 'board' as const, id: 'board', sub: 'T1' },
    ]) {
      expect(parseRef(refToString(ref))).toEqual(ref)
    }
  })

  it('rejects unknown kinds', () => {
    expect(parseRef('galaxy:x')).toBeNull()
  })
})
