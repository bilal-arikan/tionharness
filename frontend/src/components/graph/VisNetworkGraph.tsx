import { useEffect, useRef } from 'react'
import { Network, type Options, type Node, type Edge } from 'vis-network'
import { DataSet } from 'vis-data'

export type VisMode = 'relation' | 'live'

// Cap the framing zoom: vis-network's fit() zooms right up to the content, so a
// graph with only a handful of nodes ends up uncomfortably close. After every
// fit we clamp the scale down to this ceiling (kept slightly above 1 so small
// graphs still read at a natural size, never magnified).
const MAX_FIT_SCALE = 1

// fitAndCap frames the whole graph, then clamps the zoom so sparse graphs don't
// end up over-magnified. The initial framing is instant; later refits animate.
function fitAndCap(net: Network, animated: boolean) {
  const cap = () => {
    if (net.getScale() > MAX_FIT_SCALE) {
      net.moveTo({ scale: MAX_FIT_SCALE, position: net.getViewPosition() })
    }
  }
  if (animated) {
    net.fit({ animation: { duration: 400, easingFunction: 'easeInOutQuad' } })
    // Cap after the fit animation settles (getScale is mid-flight during it).
    setTimeout(cap, 440)
  } else {
    net.fit({ animation: false })
    cap()
  }
}

interface Props {
  nodes: Node[]
  edges: Edge[]
  // 'relation' = free force cloud; 'live' = board-column flow (fixed anchors at
  // top, low central gravity so columns spread horizontally).
  mode?: VisMode
  // density: 0.4 (sparse) … 2 (dense). Scales repulsion + spring length.
  density?: number
  onSelect?: (id: string | null) => void
  // When true, hovering a node dims every non-neighbour node/edge so the
  // hovered memory and its similar peers stand out (focus + context).
  highlightNeighbors?: boolean
}

// buildOptions configures the vis-network instance. Relation mode uses a
// forceAtlas2 force field (even organic spread); live mode drops central gravity
// so the fixed board-state column anchors govern the horizontal layout while
// tasks spring under their column. `density` scales repulsion/springs.
function buildOptions(density = 1, mode: VisMode = 'relation'): Options {
  const d = Math.min(2, Math.max(0.4, density))
  return {
    autoResize: true,
    nodes: {
      borderWidth: 2,
      font: { color: '#e5e7eb', size: 13, face: 'Inter, system-ui, sans-serif' },
      shadow: { enabled: true, size: 8, x: 0, y: 2, color: 'rgba(0,0,0,0.35)' },
    },
    edges: {
      color: { color: '#475569', highlight: '#94a3b8', opacity: 0.7 },
      smooth: { enabled: true, type: 'continuous', roundness: 0.5 },
      width: 1,
    },
    physics: {
      enabled: true,
      solver: 'forceAtlas2Based',
      forceAtlas2Based: {
        gravitationalConstant: (mode === 'live' ? -45 : -60) / d,
        // Live mode: near-zero central gravity so the fixed, horizontally-spread
        // column anchors (not a central pull) shape the layout.
        centralGravity: mode === 'live' ? 0.004 : 0.012 * d,
        springLength: 110 / d,
        springConstant: 0.08,
        damping: 0.4,
        avoidOverlap: mode === 'live' ? 0.6 : 0.5,
      },
      maxVelocity: 50,
      minVelocity: 0.75,
      stabilization: { enabled: true, iterations: 300, fit: true },
    },
    layout: { improvedLayout: true },
    interaction: {
      hover: true,
      tooltipDelay: 120,
      navigationButtons: false,
      keyboard: false,
    },
  }
}

// VisNetworkGraph wraps a vis-network instance (the library Agent-MCP's dashboard
// uses): a real continuous physics engine. Data is pushed onto the live DataSets
// *incrementally* (diff add/update/remove by id) so that when the graph changes —
// a task moves columns, an agent re-bonds to a new task — the physics engine
// animates the transition instead of resetting every node's position.
export function VisNetworkGraph({
  nodes,
  edges,
  mode = 'relation',
  density = 1,
  onSelect,
  highlightNeighbors = false,
}: Props) {
  const containerRef = useRef<HTMLDivElement>(null)
  const networkRef = useRef<Network | null>(null)
  const nodesDSRef = useRef<DataSet<Node> | null>(null)
  const edgesDSRef = useRef<DataSet<Edge> | null>(null)
  const modeRef = useRef(mode)
  const densityRef = useRef(density)
  const populatedRef = useRef(false)
  const onSelectRef = useRef(onSelect)
  onSelectRef.current = onSelect
  // Original edge colors, kept so blurNode can restore exactly what the mapper
  // set (per-edge opacity/width) after a hover dim.
  const baseEdgeColorRef = useRef<Map<string, Edge['color']>>(new Map())
  const highlightRef = useRef(highlightNeighbors)
  highlightRef.current = highlightNeighbors

  // Create the network once.
  useEffect(() => {
    if (!containerRef.current) return
    const nodesDS = new DataSet<Node>([])
    const edgesDS = new DataSet<Edge>([])
    nodesDSRef.current = nodesDS
    edgesDSRef.current = edgesDS
    const network = new Network(
      containerRef.current,
      { nodes: nodesDS, edges: edgesDS },
      buildOptions(densityRef.current, modeRef.current),
    )
    networkRef.current = network
    network.on('selectNode', (p: { nodes: string[] }) => onSelectRef.current?.(p.nodes[0] ?? null))
    network.on('deselectNode', () => onSelectRef.current?.(null))

    // Hover neighbour highlight: dim everything but the hovered node, its
    // direct neighbours and the edges between them. Restores on blur.
    network.on('hoverNode', (p: { node: string }) => {
      if (!highlightRef.current) return
      const nds = nodesDSRef.current
      const eds = edgesDSRef.current
      if (!nds || !eds) return
      const kept = new Set<string>([p.node, ...(network.getConnectedNodes(p.node) as string[])])
      const keptEdges = new Set<string>(network.getConnectedEdges(p.node) as string[])
      nds.update((nds.getIds() as string[]).map((id) => ({ id, opacity: kept.has(id) ? 1 : 0.12 })))
      eds.update(
        (eds.getIds() as string[]).map((id) =>
          keptEdges.has(id)
            ? { id, color: baseEdgeColorRef.current.get(id) }
            : { id, color: { color: '#334155', opacity: 0.05 } },
        ),
      )
    })
    network.on('blurNode', () => {
      if (!highlightRef.current) return
      const nds = nodesDSRef.current
      const eds = edgesDSRef.current
      if (!nds || !eds) return
      nds.update((nds.getIds() as string[]).map((id) => ({ id, opacity: 1 })))
      eds.update(
        (eds.getIds() as string[]).map((id) => ({ id, color: baseEdgeColorRef.current.get(id) })),
      )
    })
    return () => {
      network.destroy()
      networkRef.current = null
      populatedRef.current = false
    }
  }, [])

  // Incremental data sync: remove gone ids, upsert the rest. New nodes are added
  // with their full data (including the x/y seed for the physics-immune anchors);
  // EXISTING nodes are updated WITHOUT x/y so a refresh never snaps a node — most
  // importantly a user-dragged anchor — back to its seed position.
  useEffect(() => {
    const nds = nodesDSRef.current
    const eds = edgesDSRef.current
    const net = networkRef.current
    if (!nds || !eds || !net) return

    const nodeIds = new Set(nodes.map((n) => n.id as string))
    ;(nds.getIds() as string[]).forEach((id) => {
      if (!nodeIds.has(id)) nds.remove(id)
    })
    const existing = new Set(nds.getIds() as string[])
    const toAdd: Node[] = []
    const toUpdate: Node[] = []
    for (const n of nodes) {
      if (existing.has(n.id as string)) {
        // Preserve current position (physics result or user drag): drop x/y.
        const { x: _x, y: _y, ...rest } = n as Node & { x?: number; y?: number }
        toUpdate.push(rest as Node)
      } else {
        toAdd.push(n)
      }
    }
    if (toAdd.length) nds.add(toAdd)
    if (toUpdate.length) nds.update(toUpdate)

    const edgeIds = new Set(edges.map((e) => e.id as string))
    ;(eds.getIds() as string[]).forEach((id) => {
      if (!edgeIds.has(id)) {
        eds.remove(id)
        baseEdgeColorRef.current.delete(id)
      }
    })
    eds.update(edges)
    // Remember each edge's mapper-set color so hover-dim can restore it.
    for (const e of edges) baseEdgeColorRef.current.set(e.id as string, e.color)

    if (!populatedRef.current && nodes.length > 0) {
      populatedRef.current = true
      net.once('stabilizationIterationsDone', () => fitAndCap(net, false))
    }
  }, [nodes, edges])

  // Apply mode / density changes to the live instance (re-runs physics + refits).
  // Fit AFTER stabilization (not immediately) so the layout — especially live
  // mode's fixed column anchors at the top — is fully formed before framing; a
  // timeout fallback covers cases where the stabilization event doesn't fire.
  useEffect(() => {
    modeRef.current = mode
    densityRef.current = density
    const net = networkRef.current
    if (!net) return
    net.setOptions(buildOptions(density, mode))
    const fit = () => fitAndCap(net, true)
    net.once('stabilizationIterationsDone', fit)
    const t = setTimeout(fit, 1600)
    return () => clearTimeout(t)
  }, [mode, density])

  return <div ref={containerRef} className="h-full w-full" />
}
