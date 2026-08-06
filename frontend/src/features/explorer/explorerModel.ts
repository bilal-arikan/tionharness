// Explorer map graph model (see _Docs/68-OZET-HARITASI.md): a lazy-expand
// drill-down over the View layer. Each map node addresses one ViewRef; expanding
// a node fetches its structural children (GET /children) and adds one layer.
//
// This is a GRAPH, not a tree: agent→session and session(coordinator)→
// session(worker) edges can revisit a node. Layout therefore places each node
// once (first visit wins) and draws a back-edge to an already-placed node rather
// than recursing again — the visited-set that keeps a cyclic workspace from
// expanding forever.
import { MarkerType, type Edge, type Node } from '@xyflow/react'
import type { ViewKind, ViewRef } from '@/types'
import { refToString } from '@/types'

// ExpNodeData is what each map node carries. The index signature satisfies React
// Flow's Node<data> constraint.
export interface ExpNodeData {
  ref: ViewRef
  label: string
  depth: number
  expanded: boolean
  loading: boolean
  selected: boolean
  // childCount is null until the node's children have been fetched at least once.
  childCount: number | null
  // drillable is whether this kind can be expanded at all (a leaf like budget or a
  // single card cannot).
  drillable: boolean
  // dimmed is the focus+context signal: a node the current search or degree-of-
  // interest focus has pushed to the background. Rendered at reduced opacity, never
  // hidden — a faded node is still there to click.
  dimmed: boolean
  [key: string]: unknown
}

export type ExplorerRFNode = Node<ExpNodeData, 'explorer'>

// ROOT_REF seeds every map: the workspace roll-up.
export const ROOT_REF: ViewRef = { kind: 'workspace', id: 'workspace' }
export const ROOT_KEY = refToString(ROOT_REF)

// Layout spacing. A simple layered layout (depth → column, sibling index → row)
// is enough for a drill-down where each expand adds exactly one layer; it stays
// fully deterministic and needs no layout dependency (elkjs/dagre).
const COL_W = 260
const ROW_H = 72

// drillableKinds are the node kinds the map can expand into children. budget /
// tools / flowrun / schedule / logs render their breakdown inline (leaves), as
// do the TSK66 leaves artifact / automation / skill / insight; a board card
// (board ref with a sub) is a leaf too — handled in isDrillable.
const drillableKinds = new Set<ViewKind>(['workspace', 'category', 'board', 'agent', 'session'])

// isDrillable reports whether a ref can be expanded. A board WITH a sub is a
// single card — a leaf — even though the board kind is drillable.
export function isDrillable(ref: ViewRef): boolean {
  if (ref.kind === 'board' && ref.sub) return false
  return drillableKinds.has(ref.kind)
}

// nextExpandedSet applies the single-expand (accordion) rule: expanding a node
// collapses every OTHER open sibling — a node sharing the same parent — so at
// most one node per level stays open. The map reads as a drill-down, not a
// fan-out: clicking "Akışlar" closes "Oturumlar", clicking a board column closes
// its neighbour, and so on. Collapsing a node keeps everything else.
//
// parentByKey maps every node's ref-string to its parent's ref-string (the root
// has no entry). It is maintained by the hook as children are fetched, so it is
// complete for every node that can actually be expanded — a node is only
// expandable after its parent's children were fetched.
export function nextExpandedSet(
  expanded: Set<string>,
  key: string,
  parentByKey: Record<string, string>,
): Set<string> {
  const next = new Set(expanded)
  if (next.has(key)) {
    next.delete(key)
    return next
  }
  const parent = parentByKey[key]
  for (const k of next) {
    if (k !== key && parentByKey[k] === parent) next.delete(k)
  }
  next.add(key)
  return next
}

// GraphInputs is the hook state buildGraph turns into React Flow nodes/edges.
export interface GraphInputs {
  refByKey: Record<string, ViewRef>
  labelByKey: Record<string, string>
  childrenByKey: Record<string, ViewRef[]>
  expanded: Set<string>
  loading: Set<string>
  selectedKey: string | null
  // search dims every node whose label does not match (case-insensitive). Empty =
  // no search. Takes precedence over the degree-of-interest focus.
  search: string
}

// buildGraph walks the expansion from the root, placing each node once (cycle
// break) and laying out layer by layer, then applies the focus+context dimming.
// Returns React Flow nodes + edges.
export function buildGraph(g: GraphInputs): { nodes: ExplorerRFNode[]; edges: Edge[] } {
  const nodes: ExplorerRFNode[] = []
  const edges: Edge[] = []
  const placed = new Set<string>()
  const parentByKey: Record<string, string> = {}
  const rowAtDepth = new Map<number, number>()

  const visit = (ref: ViewRef, depth: number, parentKey: string | null) => {
    const key = refToString(ref)
    if (parentKey) {
      if (!(key in parentByKey)) parentByKey[key] = parentKey
      edges.push({
        id: `${parentKey}->${key}`,
        source: parentKey,
        target: key,
        markerEnd: { type: MarkerType.ArrowClosed, width: 14, height: 14 },
        style: { stroke: 'var(--color-border)' },
      })
    }
    // Already placed (a DAG re-entry or a cycle): keep the edge, stop recursing.
    if (placed.has(key)) return
    placed.add(key)

    const row = rowAtDepth.get(depth) ?? 0
    rowAtDepth.set(depth, row + 1)

    const kids = g.childrenByKey[key]
    const isExpanded = g.expanded.has(key)
    nodes.push({
      id: key,
      type: 'explorer',
      position: { x: depth * COL_W, y: row * ROW_H },
      data: {
        ref,
        label: g.labelByKey[key] ?? key,
        depth,
        expanded: isExpanded,
        loading: g.loading.has(key),
        selected: g.selectedKey === key,
        childCount: kids ? kids.length : null,
        drillable: isDrillable(ref),
        dimmed: false, // set below, once the full node set is known
      },
    })

    if (isExpanded && kids) {
      for (const childRef of kids) visit(childRef, depth + 1, key)
    }
  }

  visit(ROOT_REF, 0, null)
  applyDimming(nodes, parentByKey, g)
  dimEdges(nodes, edges)
  return { nodes, edges }
}

// applyDimming sets data.dimmed for the focus+context view:
//   - While searching, every node whose label does not contain the query fades.
//   - Otherwise, when a non-root node is selected, the degree-of-interest focus is
//     the selected node + its ancestors (the path back to the root) + its direct
//     children; everything else fades. A small or unfocused map dims nothing.
function applyDimming(
  nodes: ExplorerRFNode[],
  parentByKey: Record<string, string>,
  g: GraphInputs,
) {
  const q = g.search.trim().toLowerCase()
  if (q) {
    for (const n of nodes) n.data.dimmed = !n.data.label.toLowerCase().includes(q)
    return
  }

  const sel = g.selectedKey
  if (!sel || sel === ROOT_KEY) return // nothing selected (or the root) → no dimming

  const focus = new Set<string>([sel, ROOT_KEY])
  for (let k: string | undefined = sel; k; k = parentByKey[k]) focus.add(k) // ancestors
  for (const n of nodes) if (parentByKey[n.id] === sel) focus.add(n.id) // direct children

  for (const n of nodes) n.data.dimmed = !focus.has(n.id)
}

// dimEdges fades an edge whose target node is dimmed, so a faded branch reads as
// one unit rather than bright connectors between grey nodes.
function dimEdges(nodes: ExplorerRFNode[], edges: Edge[]) {
  const dim = new Set(nodes.filter((n) => n.data.dimmed).map((n) => n.id))
  for (const e of edges) {
    if (dim.has(e.target)) e.style = { ...e.style, opacity: 0.25 }
  }
}
