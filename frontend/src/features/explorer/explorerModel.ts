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
  focus: boolean
  overflow?: {
    side: 'parents' | 'children'
    handles: ViewHandle[]
  }
  [key: string]: unknown
}

export type ExplorerRFNode = Node<ExpNodeData, 'explorer'>

export type ExplorerNavigationKey = 'ArrowLeft' | 'ArrowRight' | 'ArrowUp' | 'ArrowDown'

// Arrow navigation follows the visual three-column hierarchy. Vertical arrows
// stay in a layer; horizontal arrows pick the nearest row in the adjacent layer.
export function nextExplorerNodeId(
  nodes: ExplorerRFNode[],
  currentId: string,
  key: ExplorerNavigationKey,
): string | null {
  const current = nodes.find((node) => node.id === currentId)
  if (!current) return null

  const vertical = key === 'ArrowUp' || key === 'ArrowDown'
  const candidates = nodes.filter((node) =>
    vertical
      ? node.data.depth === current.data.depth && node.id !== currentId
      : node.data.depth === current.data.depth + (key === 'ArrowLeft' ? -1 : 1),
  )
  if (vertical) {
    const direction = key === 'ArrowUp' ? -1 : 1
    return (
      candidates
        .filter((node) => Math.sign(node.position.y - current.position.y) === direction)
        .sort(
          (a, b) =>
            Math.abs(a.position.y - current.position.y) -
              Math.abs(b.position.y - current.position.y) || a.id.localeCompare(b.id),
        )[0]?.id ?? null
    )
  }
  return (
    candidates.sort(
      (a, b) =>
        Math.abs(a.position.y - current.position.y) - Math.abs(b.position.y - current.position.y) ||
        a.id.localeCompare(b.id),
    )[0]?.id ?? null
  )
}

export const ROOT_REF: ViewRef = { kind: 'workspace', id: 'workspace' }
export const ROOT_KEY = refToString(ROOT_REF)

const COL_W = 340
const ROW_H = 92

// Keep the canvas scannable without capping the API. Remaining handles stay in
// model output and are exposed by one synthetic node per side.
export const VISIBLE_RELATIONS_PER_SIDE = 6

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
  overflow: { parents: ViewHandle[]; children: ViewHandle[] }
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
  const allLeft = sideKeys(parentKeys)
  // A ref on both sides renders once. Parent-side placement wins deterministically.
  const allRight = sideKeys(childKeys).filter((key) => !parentKeys.has(key))
  const left = allLeft.slice(0, VISIBLE_RELATIONS_PER_SIDE)
  const right = allRight.slice(0, VISIBLE_RELATIONS_PER_SIDE)
  const overflow = {
    parents: allLeft.slice(VISIBLE_RELATIONS_PER_SIDE).map((key) => handles.get(key)!),
    children: allRight.slice(VISIBLE_RELATIONS_PER_SIDE).map((key) => handles.get(key)!),
  }
  const q = search.trim().toLowerCase()

  const makeNode = (key: string, depth: number, row: number): ExplorerRFNode => {
    const handle = handles.get(key)!
    const label = handle.label || key
    return {
      id: key,
      type: 'explorer',
      ariaRole: 'button',
      ariaLabel: `${label}. ${key === focusKey ? 'Odak düğümü. ' : ''}${key === selectedKey ? 'Seçili düğüm. ' : ''}Enter veya Boşluk seçer, Shift+Enter odaklar.`,
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
        focus: key === focusKey,
      },
    }
  }

  const overflowNode = (
    side: 'parents' | 'children',
    depth: number,
    row: number,
  ): ExplorerRFNode | null => {
    const remaining = overflow[side]
    if (remaining.length === 0) return null
    const label = `+${remaining.length} ${side === 'parents' ? 'üst' : 'alt'} bağlantı`
    return {
      id: `__overflow:${side}`,
      type: 'explorer',
      ariaRole: 'button',
      ariaLabel: `${label}. Enter veya Boşluk kalan ilişkileri açar.`,
      position: { x: depth * COL_W, y: row * ROW_H },
      data: {
        ref: neighborhood.focus.ref,
        label,
        depth,
        expanded: false,
        loading: false,
        selected: false,
        childCount: remaining.length,
        drillable: false,
        dimmed: q !== '' && !label.toLowerCase().includes(q),
        focus: false,
        overflow: { side, handles: remaining },
      },
    }
  }

  const nodes = [
    ...left.map((key, row) => makeNode(key, 0, row)),
    overflowNode('parents', 0, left.length),
    makeNode(
      focusKey,
      1,
      Math.floor(
        Math.max(
          left.length + (overflow.parents.length > 0 ? 1 : 0),
          right.length + (overflow.children.length > 0 ? 1 : 0),
          1,
        ) / 2,
      ),
    ),
    ...right.map((key, row) => makeNode(key, 2, row)),
    overflowNode('children', 2, right.length),
  ].filter((node): node is ExplorerRFNode => node !== null)
  const visibleKeys = new Set([...left, ...right])
  const edgePairs = new Set<string>()
  for (const key of visibleKeys) {
    if (parentKeys.has(key)) edgePairs.add(`${key}\u0000${focusKey}`)
    if (childKeys.has(key)) edgePairs.add(`${focusKey}\u0000${key}`)
  }
  if (parentKeys.has(focusKey) || childKeys.has(focusKey)) {
    edgePairs.add(`${focusKey}\u0000${focusKey}`)
  }
  const edges: Edge[] = [...edgePairs]
    .sort((a, b) => a.localeCompare(b))
    .map((pair) => {
      const [source, target] = pair.split('\u0000')
      const dimmed =
        nodes.find((node) => node.id === source)?.data.dimmed ||
        nodes.find((node) => node.id === target)?.data.dimmed
      const reverse = edgePairs.has(`${target}\u0000${source}`)
      const cyclic = reverse || source === target
      return {
        id: `${source}->${target}`,
        source,
        target,
        markerEnd: { type: MarkerType.ArrowClosed, width: 14, height: 14 },
        label: cyclic ? (source === target ? 'kendi üzerine döngü' : 'iki yönlü') : undefined,
        ariaLabel: cyclic
          ? source === target
            ? `${source} kendi üzerine döngü ilişkisi; kesik çizgi döngüyü belirtir`
            : `${source} ile ${target} arasında iki yönlü ilişki; kesik çizgi döngüyü belirtir`
          : `${source} öğesinden ${target} öğesine ilişki`,
        labelStyle: { fill: 'var(--color-warning)', fontSize: 10, fontWeight: 700 },
        labelBgStyle: { fill: 'var(--color-surface)', fillOpacity: 0.94 },
        labelBgPadding: [5, 3] as [number, number],
        style: {
          stroke: cyclic ? 'var(--color-warning)' : 'var(--color-border)',
          strokeDasharray: cyclic ? '7 5' : undefined,
          ...(dimmed ? { opacity: 0.25 } : {}),
        },
      }
    })

  return { nodes, edges, overflow }
}
