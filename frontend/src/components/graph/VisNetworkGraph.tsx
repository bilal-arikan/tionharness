import { useEffect, useRef } from 'react'
import { Network, type Options, type Node, type Edge } from 'vis-network'
import { DataSet } from 'vis-data'

interface Props {
  nodes: Node[]
  edges: Edge[]
  // density: 0.4 (sparse, far apart) … 2 (dense, close). Scales the physics
  // repulsion + spring length so the user can tune how tightly packed the graph
  // sits. Default 1.
  density?: number
  onSelect?: (id: string | null) => void
}

// buildOptions configures the vis-network instance to match the reference
// collaboration network: a forceAtlas2 force field (even, organic spacing the
// solver maintains automatically) on a dark canvas. This is the right layout for
// a general relationship graph — a strict hierarchy/tree only suits a DAG (the
// Flows screen), not this cyclic, partly-disconnected network. `density` scales
// repulsion/springs.
function buildOptions(density = 1): Options {
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
      // forceAtlas2Based spreads disconnected nodes into an even cloud (rather
      // than collapsing them to a clump or exploding them off-screen) — the
      // right solver for graphs that are often sparse/edgeless. avoidOverlap
      // keeps the wide task boxes from stacking.
      solver: 'forceAtlas2Based',
      forceAtlas2Based: {
        // Higher density → weaker repulsion + shorter springs → tighter packing.
        gravitationalConstant: -60 / d,
        centralGravity: 0.012 * d,
        springLength: 110 / d,
        springConstant: 0.08,
        damping: 0.4,
        avoidOverlap: 0.5,
      },
      maxVelocity: 50,
      minVelocity: 0.75,
      stabilization: { enabled: true, iterations: 320, fit: true },
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

// VisNetworkGraph wraps a vis-network instance — the same library Agent-MCP's
// dashboard uses — giving a real continuous physics engine (drag, hover, even
// organic spacing). The network is created once; node/edge data and density
// changes are pushed onto the live instance.
export function VisNetworkGraph({ nodes, edges, density = 1, onSelect }: Props) {
  const containerRef = useRef<HTMLDivElement>(null)
  const networkRef = useRef<Network | null>(null)
  const nodesDSRef = useRef<DataSet<Node> | null>(null)
  const edgesDSRef = useRef<DataSet<Edge> | null>(null)
  const densityRef = useRef(density)
  const onSelectRef = useRef(onSelect)
  onSelectRef.current = onSelect

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
      buildOptions(densityRef.current),
    )
    networkRef.current = network
    network.on('selectNode', (p: { nodes: string[] }) => onSelectRef.current?.(p.nodes[0] ?? null))
    network.on('deselectNode', () => onSelectRef.current?.(null))
    return () => {
      network.destroy()
      networkRef.current = null
    }
  }, [])

  // Push data updates onto the live DataSets (diff-free reset is fine at this
  // scale and keeps positions recomputed when the graph genuinely changes).
  useEffect(() => {
    const nds = nodesDSRef.current
    const eds = edgesDSRef.current
    if (!nds || !eds) return
    nds.clear()
    eds.clear()
    nds.add(nodes)
    eds.add(edges)
    networkRef.current?.once('stabilizationIterationsDone', () => networkRef.current?.fit({ animation: false }))
  }, [nodes, edges])

  // Apply density changes to the live instance (re-runs physics).
  useEffect(() => {
    densityRef.current = density
    const net = networkRef.current
    if (!net) return
    net.setOptions(buildOptions(density))
    setTimeout(() => net.fit({ animation: { duration: 300, easingFunction: 'easeInOutQuad' } }), 60)
  }, [density])

  return <div ref={containerRef} className="h-full w-full" />
}
