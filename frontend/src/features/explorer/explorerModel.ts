import { MarkerType, type Edge, type Node } from '@xyflow/react'
import type { ViewHandle, ViewNeighborhoodResult, ViewRef } from '@/types'
import { refToString } from '@/types'

export interface ExpNodeData {
  ref: ViewRef
  label: string
  depth: number
  expanded: boolean
  loading: boolean
  selected: boolean
  childCount: number | null
  drillable: boolean
  dimmed: boolean
  [key: string]: unknown
}

export type ExplorerRFNode = Node<ExpNodeData, 'explorer'>

export const ROOT_REF: ViewRef = { kind: 'workspace', id: 'workspace' }
export const ROOT_KEY = refToString(ROOT_REF)

const COL_W = 260
const ROW_H = 72

export interface FocusGraphInputs {
  neighborhood: ViewNeighborhoodResult
  selectedKey: string | null
  search: string
  loading: boolean
}

function compareHandles(a: ViewHandle, b: ViewHandle): number {
  return refToString(a.ref).localeCompare(refToString(b.ref))
}

// Build only direct parent -> focus -> direct child relationships. Ref identity
// owns node de-duplication; directed endpoints own edge de-duplication.
export function buildFocusGraph({ neighborhood, selectedKey, search, loading }: FocusGraphInputs): {
  nodes: ExplorerRFNode[]
  edges: Edge[]
} {
  const focusKey = refToString(neighborhood.focus.ref)
  const parents = [...neighborhood.parents].sort(compareHandles)
  const children = [...neighborhood.children].sort(compareHandles)
  const handles = new Map<string, ViewHandle>([[focusKey, neighborhood.focus]])
  for (const handle of [...parents, ...children]) {
    const key = refToString(handle.ref)
    if (!handles.has(key)) handles.set(key, handle)
  }

  const parentKeys = new Set(parents.map((handle) => refToString(handle.ref)))
  const childKeys = new Set(children.map((handle) => refToString(handle.ref)))
  const sideKeys = (side: Set<string>) =>
    [...side].filter((key) => key !== focusKey).sort((a, b) => a.localeCompare(b))
  const left = sideKeys(parentKeys)
  // A ref on both sides renders once. Parent-side placement wins deterministically.
  const right = sideKeys(childKeys).filter((key) => !parentKeys.has(key))
  const q = search.trim().toLowerCase()

  const makeNode = (key: string, depth: number, row: number): ExplorerRFNode => {
    const handle = handles.get(key)!
    const label = handle.label || key
    return {
      id: key,
      type: 'explorer',
      position: { x: depth * COL_W, y: row * ROW_H },
      data: {
        ref: handle.ref,
        label,
        depth,
        expanded: key === focusKey,
        loading: key === focusKey && loading,
        selected: key === selectedKey,
        childCount: key === focusKey ? children.length : null,
        drillable: true,
        dimmed: q !== '' && !label.toLowerCase().includes(q),
      },
    }
  }

  const nodes = [
    ...left.map((key, row) => makeNode(key, 0, row)),
    makeNode(focusKey, 1, Math.floor(Math.max(left.length, right.length, 1) / 2)),
    ...right.map((key, row) => makeNode(key, 2, row)),
  ]
  const edgePairs = new Set<string>()
  for (const key of parentKeys) edgePairs.add(`${key}\u0000${focusKey}`)
  for (const key of childKeys) edgePairs.add(`${focusKey}\u0000${key}`)
  const edges: Edge[] = [...edgePairs]
    .sort((a, b) => a.localeCompare(b))
    .map((pair) => {
      const [source, target] = pair.split('\u0000')
      const dimmed =
        nodes.find((node) => node.id === source)?.data.dimmed ||
        nodes.find((node) => node.id === target)?.data.dimmed
      return {
        id: `${source}->${target}`,
        source,
        target,
        markerEnd: { type: MarkerType.ArrowClosed, width: 14, height: 14 },
        style: { stroke: 'var(--color-border)', ...(dimmed ? { opacity: 0.25 } : {}) },
      }
    })

  return { nodes, edges }
}
