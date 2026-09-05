import { useEffect, useRef } from 'react'
import { Network, type Options, type Node, type Edge } from 'vis-network'
import { DataSet } from 'vis-data'
import {
  readNetworkLayout,
  writeNetworkLayout,
  type NetworkPositions,
  type NetworkViewport,
} from './networkLayoutStorage'
import {
  captureNetworkPositions,
  positionCoordinates,
  restoreNetworkVelocities,
  wakeSettledNetwork,
} from './networkPhysicsState'

// 'relation' = free force cloud (forceAtlas2); 'live' = board-column flow;
// 'tree' = a hierarchy: attraction only along edges (parent <-> child springs)
// plus SHORT-RANGE repulsion between neighbours, so a subtree feels only its
// own parent and its siblings instead of every node on the canvas.
export type VisMode = 'relation' | 'live' | 'tree'

// Cap the framing zoom: vis-network's fit() zooms right up to the content, so a
// graph with only a handful of nodes ends up uncomfortably close. After every
// fit we clamp the scale down to this ceiling (kept slightly above 1 so small
// graphs still read at a natural size, never magnified).
const MAX_FIT_SCALE = 1

// fitAndCap frames the whole graph, then clamps the zoom so sparse graphs don't
// end up over-magnified. The initial framing is instant; later refits animate.
function fitAndCap(net: Network, animated: boolean): () => void {
  const cap = () => {
    if (net.getScale() > MAX_FIT_SCALE) {
      net.moveTo({ scale: MAX_FIT_SCALE, position: net.getViewPosition() })
    }
  }
  if (animated) {
    net.fit({ animation: { duration: 400, easingFunction: 'easeInOutQuad' } })
    // Cap after the fit animation settles (getScale is mid-flight during it).
    const timer = setTimeout(cap, 440)
    return () => clearTimeout(timer)
  } else {
    net.fit({ animation: false })
    cap()
    return () => {}
  }
}

function captureNetworkViewport(net: Network): NetworkViewport {
  const scale = net.getScale()
  const position = net.getViewPosition()
  if (
    !Number.isFinite(scale) ||
    scale <= 0 ||
    !Number.isFinite(position.x) ||
    !Number.isFinite(position.y)
  ) {
    throw new Error('vis-network returned an invalid viewport')
  }
  return { scale, position }
}

interface Props {
  workspaceId: string
  nodes: Node[]
  edges: Edge[]
  canonicalNodeIds: readonly string[]
  canonicalReady: boolean
  // 'relation' = free force cloud; 'live' = board-column flow (fixed anchors at
  // top, low central gravity so columns spread horizontally).
  mode?: VisMode
  // density: 0.4 (sparse) … 2 (dense). Scales repulsion + spring length.
  density?: number
  onSelect?: (id: string | null) => void
  // When true, hovering a node dims every non-neighbour node/edge so the
  // hovered memory and its similar peers stand out (focus + context).
  highlightNeighbors?: boolean
  // Low-power rendering (phones): drop shadows, curved edges, improvedLayout and
  // hover to keep pan/zoom smooth. See buildOptions.
  lite?: boolean
  // Layout persistence namespace. Defaults to the workspace id; a second screen
  // sharing this component (the Explorer map) passes its own key so the two
  // screens' saved positions never garbage-collect each other.
  layoutId?: string
  // Camera focus request: when `focusNodeId` names a node in the graph the
  // camera glides to it and vis selects it. `focusTick` lets the host re-request
  // the same node (a second click on it) — the effect keys on both.
  focusNodeId?: string | null
  focusTick?: number
  // Double click on a node (vis 'doubleClick' with a node under the pointer).
  onNodeDoubleClick?: (id: string) => void
  // Let the simulation come to rest (minVelocity > 0, stronger damping) instead
  // of the live screen's perpetual motion. The Explorer map wants a still
  // picture once the physics has done its job.
  settle?: boolean
}

interface ThemeColors {
  border: string
  text: string
  textDim: string
}

function resolveThemeColors(): ThemeColors {
  const styles = getComputedStyle(document.documentElement)
  const read = (token: string) => {
    const value = styles.getPropertyValue(token).trim()
    if (!value) throw new Error(`Theme token ${token} did not resolve`)
    return value
  }
  return {
    border: read('--color-border'),
    text: read('--color-text'),
    textDim: read('--color-text-dim'),
  }
}

// buildOptions configures the vis-network instance. Relation mode uses a
// forceAtlas2 force field (even organic spread); live mode drops central gravity
// so the fixed board-state column anchors govern the horizontal layout while
// tasks spring under their column. `density` scales repulsion/springs.
// `lite` trims the per-frame canvas cost for low-power/touch devices (phones):
// node shadows and curved edges are the two biggest repaint costs while panning /
// zooming, and improvedLayout + a high stabilization count make first paint janky.
// Dropping them keeps the same graph, just cheaper to render.
function buildOptions(
  colors: ThemeColors,
  density = 1,
  mode: VisMode = 'relation',
  lite = false,
  physicsEnabled = false,
  improvedLayoutEnabled = !lite,
  stabilizationFit = true,
  settle = false,
): Options {
  const d = Math.min(2, Math.max(0.4, density))
  const tree = mode === 'tree'
  return {
    autoResize: true,
    nodes: {
      borderWidth: 2,
      font: { color: colors.text, size: 13, face: 'Inter, system-ui, sans-serif' },
      shadow: lite
        ? { enabled: false }
        : { enabled: true, size: 8, x: 0, y: 2, color: colors.border },
    },
    edges: {
      color: { color: colors.border, highlight: colors.textDim, opacity: 0.7 },
      // Straight edges on mobile: continuous smoothing recomputes bezier control
      // points every frame, which is the main pan/zoom stutter on phones.
      smooth: lite ? false : { enabled: true, type: 'continuous', roundness: 0.5 },
      width: 1,
    },
    physics: {
      enabled: physicsEnabled,
      solver: tree ? 'repulsion' : 'forceAtlas2Based',
      // Tree mode: no long-range field at all. Springs (per-edge `length`) hold
      // each child to its parent; `nodeDistance` is the radius inside which two
      // nodes push apart, so only siblings and near neighbours interact.
      repulsion: {
        nodeDistance: 140 / d,
        centralGravity: 0,
        springLength: 130 / d,
        springConstant: 0.05,
        damping: 0.6,
      },
      forceAtlas2Based: {
        // Stronger repulsion in live mode so the many nodes sharing one anchor
        // (run-history cards on "Geçmiş", instances on "Çalışıyor") push apart
        // instead of stacking on top of each other.
        gravitationalConstant: (mode === 'live' ? -70 : -60) / d,
        // Live mode: near-zero central gravity so the fixed, horizontally-spread
        // column anchors (not a central pull) shape the layout.
        centralGravity: mode === 'live' ? 0.004 : 0.012 * d,
        springLength: 110 / d,
        springConstant: 0.08,
        // A settling graph damps harder so the light leaf nodes stop bouncing
        // around their hub instead of trading energy back and forth forever.
        damping: settle ? 0.55 : 0.4,
        // avoidOverlap pushed to the max in live mode: box cards with long titles
        // were overlapping at the shared anchors; 1 keeps them clear of each other.
        avoidOverlap: mode === 'live' ? 1 : 0.6,
      },
      maxVelocity: 50,
      // Live mode keeps ticking instead of declaring itself settled shortly
      // after a restored layout receives its wake velocity. A settling graph
      // uses the vis default threshold: once every node is slower than this the
      // simulation stops on its own and only a drag or new data wakes it.
      minVelocity: settle ? 0.75 : 0,
      // Fewer settle iterations on mobile so the initial simulation burst is short.
      stabilization: { enabled: true, iterations: lite ? 120 : 300, fit: stabilizationFit },
    },
    // improvedLayout is constructor-only. Restored and low-power layouts skip it.
    layout: { improvedLayout: improvedLayoutEnabled },
    interaction: {
      // Touch devices have no hover; disabling it drops the neighbour-dim repaint.
      hover: !lite,
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
  workspaceId,
  nodes,
  edges,
  canonicalNodeIds,
  canonicalReady,
  mode = 'relation',
  density = 1,
  onSelect,
  highlightNeighbors = false,
  lite = false,
  layoutId,
  focusNodeId = null,
  focusTick = 0,
  onNodeDoubleClick,
  settle = false,
}: Props) {
  const layoutKey = layoutId ?? workspaceId
  const containerRef = useRef<HTMLDivElement>(null)
  const networkRef = useRef<Network | null>(null)
  const nodesDSRef = useRef<DataSet<Node> | null>(null)
  const edgesDSRef = useRef<DataSet<Edge> | null>(null)
  const modeRef = useRef(mode)
  const densityRef = useRef(density)
  const liteRef = useRef(lite)
  const settleRef = useRef(settle)
  const populatedRef = useRef(false)
  const initialLayoutRef = useRef(readNetworkLayout(layoutKey))
  const persistedPositionsRef = useRef<NetworkPositions>(initialLayoutRef.current.positions)
  const preserveViewportRef = useRef(initialLayoutRef.current.viewport !== undefined)
  const physicsActiveRef = useRef(false)
  const runtimePositionsRef = useRef<NetworkPositions>({})
  const nodesRef = useRef(nodes)
  const edgesRef = useRef(edges)
  const canonicalNodeIdsRef = useRef(canonicalNodeIds)
  const canonicalReadyRef = useRef(canonicalReady)
  const setupGenerationByWorkspaceRef = useRef(new Map<string, number>())
  const stabilizationCleanupRef = useRef<() => void>(() => {})
  const temporaryFixedRef = useRef<Map<string, Node['fixed']>>(new Map())
  const fitCleanupRef = useRef<() => void>(() => {})
  const onSelectRef = useRef(onSelect)
  const onNodeDoubleClickRef = useRef(onNodeDoubleClick)
  // Original edge colors, kept so blurNode can restore exactly what the mapper
  // set (per-edge opacity/width) after a hover dim.
  const baseEdgeColorRef = useRef<Map<string, Edge['color']>>(new Map())
  const highlightRef = useRef(highlightNeighbors)
  const dimmedEdgeColorRef = useRef('')
  // Post-commit assignment: these refs exist so the vis-network callbacks (bound
  // once, outside React) always see the latest props. Handlers fire after commit.
  useEffect(() => {
    liteRef.current = lite
    settleRef.current = settle
    nodesRef.current = nodes
    edgesRef.current = edges
    canonicalNodeIdsRef.current = canonicalNodeIds
    canonicalReadyRef.current = canonicalReady
    onSelectRef.current = onSelect
    onNodeDoubleClickRef.current = onNodeDoubleClick
    highlightRef.current = highlightNeighbors
  })

  // Create the network once.
  useEffect(() => {
    if (!containerRef.current) return
    const setupGenerations = setupGenerationByWorkspaceRef.current
    const generation = (setupGenerations.get(layoutKey) ?? 0) + 1
    setupGenerations.set(layoutKey, generation)
    const persistedLayout = readNetworkLayout(layoutKey)
    persistedPositionsRef.current = persistedLayout.positions
    preserveViewportRef.current = persistedLayout.viewport !== undefined
    physicsActiveRef.current = false
    let knownPositions = persistedPositionsRef.current
    runtimePositionsRef.current = {}
    const baseEdgeColors = baseEdgeColorRef.current
    const colors = resolveThemeColors()
    dimmedEdgeColorRef.current = colors.border
    // Seed the DataSet before constructing Network. Adding restored x/y only in
    // the later sync effect lets vis-network initialize its internal body at
    // unrelated coordinates even when physics is disabled.
    const initialNodes = (canonicalReadyRef.current ? nodesRef.current : []).map((node) => {
      const saved = persistedPositionsRef.current[node.id as string]
      return saved ? { ...node, ...positionCoordinates(saved) } : node
    })
    const hasRestoredVisibleNode = initialNodes.some(
      (node) => persistedPositionsRef.current[node.id as string] !== undefined,
    )
    const nodesDS = new DataSet<Node>(initialNodes)
    const edgesDS = new DataSet<Edge>(canonicalReadyRef.current ? edgesRef.current : [])
    nodesDSRef.current = nodesDS
    edgesDSRef.current = edgesDS
    const network = new Network(
      containerRef.current,
      { nodes: nodesDS, edges: edgesDS },
      buildOptions(
        colors,
        densityRef.current,
        modeRef.current,
        liteRef.current,
        preserveViewportRef.current,
        !liteRef.current && !hasRestoredVisibleNode,
        !preserveViewportRef.current,
        settleRef.current,
      ),
    )
    networkRef.current = network
    // improvedLayout can shift predefined coordinates inside the constructor.
    // Reapply persisted positions through vis-network's public world-space API.
    for (const node of initialNodes) {
      const saved = persistedPositionsRef.current[node.id as string]
      if (saved) network.moveNode(node.id as string, saved.x, saved.y)
    }
    let runtimeViewport: NetworkViewport | undefined = persistedLayout.viewport
    const captureRuntimeViewport = () => {
      runtimeViewport = captureNetworkViewport(network)
    }
    const restorePersistedViewport = () => {
      if (!persistedLayout.viewport) return
      network.moveTo({ ...persistedLayout.viewport, animation: false })
      runtimeViewport = persistedLayout.viewport
    }
    let viewportRestoreActive = true
    const restorePersistedViewportAfterResize = () => {
      // Canvas emits resize before applying its cached camera state. Wait until
      // that call stack completes, then make the persisted camera authoritative.
      queueMicrotask(() => {
        if (viewportRestoreActive) restorePersistedViewport()
      })
    }
    restorePersistedViewport()
    // vis-network can resize its canvas during first paint and while releasing
    // its hidden stabilization batch. Reapply after each one-time initialization
    // boundary so none of them can alter the restored camera.
    if (persistedLayout.viewport) {
      network.once('afterDrawing', restorePersistedViewport)
      network.once('resize', restorePersistedViewportAfterResize)
    }
    const completeInitialViewport = persistedLayout.viewport
      ? restorePersistedViewport
      : captureRuntimeViewport
    network.once('stabilizationIterationsDone', completeInitialViewport)
    network.on('selectNode', (p: { nodes: string[] }) => onSelectRef.current?.(p.nodes[0] ?? null))
    network.on('deselectNode', () => onSelectRef.current?.(null))
    network.on('doubleClick', (p: { nodes: string[] }) => {
      const id = p.nodes[0]
      if (id !== undefined) onNodeDoubleClickRef.current?.(id)
    })
    // Rest when stable (settle mode): once vis reports the simulation settled
    // the physics is switched OFF, so nothing — a refresh, a re-styled node, a
    // theme swap — can nudge the picture. A drag switches it back on so the
    // neighbours react, and the next 'stabilized' turns it off again. New
    // nodes go through settleNewNodes, which enables physics itself.
    network.on('stabilized', () => {
      if (!settleRef.current || !physicsActiveRef.current) return
      network.setOptions({ physics: { enabled: false } })
      physicsActiveRef.current = false
    })
    network.on('dragStart', (p: { nodes: string[] }) => {
      if (!settleRef.current || p.nodes.length === 0 || physicsActiveRef.current) return
      network.setOptions({ physics: { enabled: true } })
      physicsActiveRef.current = true
      network.startSimulation()
    })
    network.on('zoom', captureRuntimeViewport)
    network.on('dragEnd', captureRuntimeViewport)
    const capturePositions = (): NetworkPositions => {
      const ids = nodesDS.getIds() as string[]
      return ids.length === 0 ? {} : captureNetworkPositions(network, ids, physicsActiveRef.current)
    }
    const persistPositions = (
      positions: NetworkPositions,
      nodeIds: readonly string[],
      physicsActive: boolean,
    ) => {
      const layout = writeNetworkLayout(
        layoutKey,
        { positions, physicsActive, viewport: runtimeViewport },
        localStorage,
        nodeIds,
        knownPositions,
      )
      persistedPositionsRef.current = layout.positions
      knownPositions = Object.fromEntries(
        nodeIds.flatMap((id) => {
          const position = persistedPositionsRef.current[id]
          return position ? [[id, position] as const] : []
        }),
      )
    }
    const handlePageHide = () => {
      if (!canonicalReadyRef.current) return
      captureRuntimeViewport()
      persistPositions(
        {
          ...persistedPositionsRef.current,
          ...runtimePositionsRef.current,
          ...capturePositions(),
        },
        canonicalNodeIdsRef.current,
        physicsActiveRef.current,
      )
    }
    window.addEventListener('pagehide', handlePageHide)

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
            : { id, color: { color: dimmedEdgeColorRef.current, opacity: 0.05 } },
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
      // Capture before tearing down vis-network. Deferring only the storage write
      // lets React StrictMode's immediate setup-cleanup-setup replay invalidate
      // its seed snapshot, while a real unmount/workspace change still persists.
      const ready = canonicalReadyRef.current
      const physicsActive = physicsActiveRef.current
      const positions = {
        ...persistedPositionsRef.current,
        ...runtimePositionsRef.current,
        ...capturePositions(),
      }
      const viewport = runtimeViewport
      const nodeIds = [...canonicalNodeIdsRef.current]
      viewportRestoreActive = false
      window.removeEventListener('pagehide', handlePageHide)
      network.off('afterDrawing', restorePersistedViewport)
      network.off('resize', restorePersistedViewportAfterResize)
      network.off('stabilizationIterationsDone', completeInitialViewport)
      network.off('zoom', captureRuntimeViewport)
      network.off('dragEnd', captureRuntimeViewport)
      stabilizationCleanupRef.current()
      stabilizationCleanupRef.current = () => {}
      temporaryFixedRef.current.clear()
      fitCleanupRef.current()
      fitCleanupRef.current = () => {}
      network.destroy()
      networkRef.current = null
      nodesDSRef.current = null
      edgesDSRef.current = null
      populatedRef.current = false
      baseEdgeColors.clear()
      queueMicrotask(() => {
        if (!ready || setupGenerations.get(layoutKey) !== generation) return
        writeNetworkLayout(
          layoutKey,
          { positions, physicsActive, viewport },
          localStorage,
          nodeIds,
          knownPositions,
        )
      })
    }
  }, [layoutKey])

  // Theme presets are applied as inline root tokens; data-theme additionally
  // distinguishes light mode. Re-resolve both without polling when either changes.
  useEffect(() => {
    const root = document.documentElement
    const observer = new MutationObserver(() => {
      const net = networkRef.current
      if (!net) return
      const colors = resolveThemeColors()
      dimmedEdgeColorRef.current = colors.border
      net.setOptions(
        buildOptions(
          colors,
          densityRef.current,
          modeRef.current,
          liteRef.current,
          physicsActiveRef.current,
          false,
          !preserveViewportRef.current,
          settleRef.current,
        ),
      )
    })
    observer.observe(root, { attributes: true, attributeFilter: ['style', 'data-theme'] })
    return () => observer.disconnect()
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
    if (!canonicalReady) return

    const existingIds = nds.getIds() as string[]
    // Constructor-seeded nodes are not persisted positions. Capture runtime
    // coordinates only after the initial restore/new-node decision has run.
    if (populatedRef.current && existingIds.length > 0) {
      runtimePositionsRef.current = {
        ...runtimePositionsRef.current,
        ...captureNetworkPositions(net, existingIds, physicsActiveRef.current),
      }
    }
    const knownPositions = {
      ...persistedPositionsRef.current,
      ...runtimePositionsRef.current,
    }
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
        toUpdate.push(
          temporaryFixedRef.current.has(n.id as string)
            ? ({ ...rest, fixed: { x: true, y: true } } as Node)
            : (rest as Node),
        )
      } else {
        const saved = knownPositions[n.id as string]
        toAdd.push(saved ? { ...n, ...positionCoordinates(saved) } : n)
      }
    }
    if (toAdd.length) nds.add(toAdd)
    if (toUpdate.length) nds.update(toUpdate)
    for (const node of toAdd) {
      const saved = knownPositions[node.id as string]
      if (saved) net.moveNode(node.id as string, saved.x, saved.y)
    }

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

    const settleNewNodes = (
      newNodeIds: string[],
      animated: boolean,
      resumedPositions?: NetworkPositions,
      completeAfterSettle = true,
    ) => {
      stabilizationCleanupRef.current()
      fitCleanupRef.current()
      const newIds = new Set(newNodeIds)
      const fixedBefore = new Map<string, Node['fixed']>()
      for (const id of nds.getIds() as string[]) {
        if (newIds.has(id)) continue
        const node = nds.get(id) as Node | null
        fixedBefore.set(id, node?.fixed)
        nds.update({ id, fixed: { x: true, y: true } })
      }
      temporaryFixedRef.current = fixedBefore

      let completed = false
      let timer: ReturnType<typeof setTimeout> | null = null
      const restoreTemporaryFixed = () => {
        const currentIds = new Set(nds.getIds() as string[])
        nds.update(
          [...fixedBefore]
            .filter(([id]) => currentIds.has(id))
            .map(([id, fixed]) => ({ id, fixed })),
        )
        if (temporaryFixedRef.current === fixedBefore) temporaryFixedRef.current = new Map()
      }
      const complete = () => {
        if (completed) return
        completed = true
        if (timer) clearTimeout(timer)
        net.off('stabilizationIterationsDone', complete)
        // The short settling phase only controls temporary anchors. Physics stays
        // enabled for the lifetime of the network screen.
        restoreTemporaryFixed()
        const ids = nds.getIds() as string[]
        runtimePositionsRef.current = {
          ...runtimePositionsRef.current,
          ...(net.getPositions(ids) as NetworkPositions),
        }
        fitCleanupRef.current = fitAndCap(net, animated)
      }
      stabilizationCleanupRef.current = () => {
        if (timer) clearTimeout(timer)
        net.off('stabilizationIterationsDone', complete)
        if (!completed) {
          net.stopSimulation()
          net.setOptions({ physics: { enabled: false } })
          physicsActiveRef.current = false
          restoreTemporaryFixed()
        }
        completed = true
      }
      net.setOptions({ physics: { enabled: true } })
      if (resumedPositions) {
        restoreNetworkVelocities(net, resumedPositions)
        wakeSettledNetwork(net, resumedPositions)
      }
      if (completeAfterSettle) {
        net.once('stabilizationIterationsDone', complete)
        timer = setTimeout(complete, 1600)
      }
      physicsActiveRef.current = true
      net.startSimulation()
    }

    if (!populatedRef.current && nodes.length > 0) {
      populatedRef.current = true
      // Every screen entry starts continuous physics from the persisted world
      // coordinates. Active snapshots restore velocity; settled snapshots receive
      // a small deterministic wake velocity before re-entering the force field.
      settleNewNodes(
        nodes.map((node) => node.id as string),
        false,
        knownPositions,
        false,
      )
    } else {
      const newNodeIds = toAdd
        .filter((node) => knownPositions[node.id as string] === undefined)
        .map((node) => node.id as string)
      if (newNodeIds.length > 0) settleNewNodes(newNodeIds, true)
    }
  }, [nodes, edges, layoutKey, canonicalNodeIds, canonicalReady])

  // Camera focus. Runs after the data sync above (effect order), so a node that
  // arrived in the same render is already in the DataSet. Zoom never drops below
  // the current scale — focusing should bring a node closer, not zoom out — and
  // the resulting camera is captured so it persists like a manual pan.
  const handledFocusRef = useRef('')
  useEffect(() => {
    const net = networkRef.current
    const nds = nodesDSRef.current
    if (!net || !nds || !focusNodeId) return
    const request = `${focusTick}:${focusNodeId}`
    // Each request is honoured once. `nodes` is a dependency only so a request
    // made before its node arrived (a deep link) fires once the node exists —
    // a later data refresh must not yank the camera back.
    if (handledFocusRef.current === request || !nds.get(focusNodeId)) return
    handledFocusRef.current = request
    net.selectNodes([focusNodeId])
    net.focus(focusNodeId, {
      scale: Math.max(net.getScale(), 1),
      animation: { duration: 400, easingFunction: 'easeInOutQuad' },
    })
  }, [focusNodeId, focusTick, nodes])

  // Visual option changes preserve the current physics state and restored layout.
  useEffect(() => {
    modeRef.current = mode
    densityRef.current = density
    const net = networkRef.current
    if (!net) return
    net.setOptions(
      buildOptions(
        resolveThemeColors(),
        density,
        mode,
        lite,
        physicsActiveRef.current,
        false,
        !preserveViewportRef.current,
        settle,
      ),
    )
  }, [mode, density, lite, settle])

  return <div ref={containerRef} className="h-full w-full" />
}
