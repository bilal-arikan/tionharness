import type { ViewGraphResult } from '@/types'
import { refToString } from '@/types'

// Radial seed layout for the Explorer network. vis-network's force field does
// the real work; this only hands it a sensible starting picture — root in the
// middle, the eleven buckets on a ring, every subtree fanned out inside its
// bucket's angular sector — so the first frames do not untangle a random cloud
// and the persisted layout (once the user has one) is what the seeds yield to.

export interface SeedPosition {
  x: number
  y: number
}

export interface SeedLayout {
  positions: Record<string, SeedPosition>
  // BFS depth from the root (root = 0, buckets = 1). Nodes unreachable from the
  // root are absent from both maps; the caller treats them as depth 2.
  depth: Record<string, number>
}

// Ring radius of the buckets and the extra radius per further depth.
export const SEED_RING_RADIUS = 340
export const SEED_RING_STEP = 190

export function seedLayout(graph: ViewGraphResult, rootKey: string): SeedLayout {
  const children = new Map<string, string[]>()
  for (const edge of graph.edges) {
    const source = refToString(edge.source)
    const target = refToString(edge.target)
    if (source === target) continue
    const list = children.get(source) ?? []
    list.push(target)
    children.set(source, list)
  }

  const depth: Record<string, number> = {}
  const positions: Record<string, SeedPosition> = {}
  // Angular sector [from, to] owned by each placed node; its children split it.
  const sector = new Map<string, [number, number]>()

  depth[rootKey] = 0
  positions[rootKey] = { x: 0, y: 0 }
  sector.set(rootKey, [-Math.PI / 2, (3 * Math.PI) / 2])

  const queue = [rootKey]
  while (queue.length > 0) {
    const current = queue.shift()!
    const [from, to] = sector.get(current)!
    const kids = (children.get(current) ?? []).filter((key) => depth[key] === undefined)
    if (kids.length === 0) continue
    const span = (to - from) / kids.length
    const radius = SEED_RING_RADIUS + SEED_RING_STEP * depth[current]
    kids.forEach((key, index) => {
      const a = from + span * index
      const b = a + span
      const angle = (a + b) / 2
      depth[key] = depth[current] + 1
      positions[key] = {
        x: Math.round(Math.cos(angle) * radius),
        y: Math.round(Math.sin(angle) * radius),
      }
      sector.set(key, [a, b])
      queue.push(key)
    })
  }
  return { positions, depth }
}
