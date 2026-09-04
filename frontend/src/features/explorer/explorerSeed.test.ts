import { describe, expect, it } from 'vitest'
import type { ViewGraphResult, ViewRef } from '@/types'
import { seedLayout, SEED_RING_RADIUS, SEED_RING_STEP } from './explorerSeed'

const ref = (kind: ViewRef['kind'], id: string): ViewRef => ({ kind, id })
const ROOT = 'workspace:workspace'

function graph(edges: [ViewRef, ViewRef][], extraNodes: ViewRef[] = []): ViewGraphResult {
  const seen = new Map<string, ViewRef>()
  for (const [a, b] of edges) {
    seen.set(`${a.kind}:${a.id}`, a)
    seen.set(`${b.kind}:${b.id}`, b)
  }
  for (const n of extraNodes) seen.set(`${n.kind}:${n.id}`, n)
  return {
    nodes: [...seen.values()].map((r) => ({ label: r.id, ref: r })),
    edges: edges.map(([source, target]) => ({ source, target })),
  }
}

describe('seedLayout', () => {
  it('puts the root at the origin, buckets on the ring and members one step out', () => {
    const root = ref('workspace', 'workspace')
    const a = ref('category', 'a')
    const b = ref('category', 'b')
    const m = ref('session', 'm')
    const { positions, depth } = seedLayout(
      graph([
        [root, a],
        [root, b],
        [a, m],
      ]),
      ROOT,
    )

    expect(positions[ROOT]).toEqual({ x: 0, y: 0 })
    expect(depth).toEqual({ [ROOT]: 0, 'category:a': 1, 'category:b': 1, 'session:m': 2 })
    const radius = (p: { x: number; y: number }) => Math.hypot(p.x, p.y)
    expect(radius(positions['category:a'])).toBeCloseTo(SEED_RING_RADIUS, 0)
    expect(radius(positions['category:b'])).toBeCloseTo(SEED_RING_RADIUS, 0)
    expect(radius(positions['session:m'])).toBeCloseTo(SEED_RING_RADIUS + SEED_RING_STEP, 0)
    // Two buckets sit on opposite sides of the ring.
    expect(positions['category:a'].x).toBeCloseTo(-positions['category:b'].x, 0)
  })

  it('keeps a member inside its bucket sector and assigns each node once', () => {
    const root = ref('workspace', 'workspace')
    const buckets = ['a', 'b', 'c', 'd'].map((id) => ref('category', id))
    const m = ref('session', 'm')
    const edges: [ViewRef, ViewRef][] = buckets.map((b) => [root, b])
    edges.push([buckets[2], m])
    const { positions, depth } = seedLayout(graph(edges), ROOT)
    const bucket = positions['category:c']
    const member = positions['session:m']
    const angle = (p: { x: number; y: number }) => Math.atan2(p.y, p.x)
    expect(Math.abs(angle(bucket) - angle(member))).toBeLessThan(Math.PI / 4)
    expect(Object.keys(depth)).toHaveLength(6)
  })

  it('terminates on cycles and self-loops and ignores unreachable nodes', () => {
    const root = ref('workspace', 'workspace')
    const a = ref('category', 'sessions')
    const x = ref('session', 'x')
    const y = ref('session', 'y')
    const loner = ref('session', 'loner')
    const { positions, depth } = seedLayout(
      graph(
        [
          [root, a],
          [a, x],
          [a, y],
          [x, y],
          [y, x],
          [x, x],
        ],
        [loner],
      ),
      ROOT,
    )
    expect(depth['session:x']).toBe(2)
    expect(depth['session:y']).toBe(2)
    expect(positions['session:loner']).toBeUndefined()
    expect(depth['session:loner']).toBeUndefined()
  })
})
