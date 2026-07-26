import { useCallback, useMemo, useState } from 'react'
import {
  ReactFlow,
  ReactFlowProvider,
  Background,
  Controls,
  MiniMap,
  Panel,
  MarkerType,
  useReactFlow,
  addEdge,
  type Connection,
  type Edge,
  type EdgeChange,
  type NodeChange,
  type NodeTypes,
  type OnSelectionChangeParams,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import './flowCanvas.css'
import type { Agent, FlowNodeType } from '@/types'
import type { FlowRFNode } from './flowGraph'
import { AgentsContext, NodeActionsContext, chromeFor, type NodeActions } from './nodeStyles'
import { AgentNode } from './AgentNode'
import { BranchNode } from './BranchNode'
import { ParallelNode } from './ParallelNode'
import { DelayNode } from './DelayNode'
import { TransformNode } from './TransformNode'
import { LoopNode } from './LoopNode'

// CanvasTools is a small in-canvas toolbar (top-right Panel). It lives inside
// ReactFlowProvider so it can use the programmatic viewport API. "Otomatik diz"
// asks the parent to re-layout, then re-centers once positions settle (editable
// only). The mini-map show/hide toggle is always available. (Fit/zoom already
// live in the bottom-left Controls, so there's no separate center button.)
function CanvasTools({
  onAutoLayout,
  showMinimap,
  onToggleMinimap,
}: {
  onAutoLayout?: () => void
  showMinimap: boolean
  onToggleMinimap: () => void
}) {
  const { fitView } = useReactFlow()
  return (
    <Panel position="top-right">
      <div className="flex gap-1 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-1 text-xs shadow-lg">
        {onAutoLayout && (
          <button
            data-testid="flow-canvas-auto-layout"
            onClick={() => {
              onAutoLayout()
              setTimeout(() => fitView({ padding: 0.2, duration: 300 }), 60)
            }}
            className="rounded px-2 py-1 hover:bg-[var(--color-surface-2)]"
            title="Düğümleri otomatik diz"
          >
            ▦ Oto diz
          </button>
        )}
        <button
          data-testid="flow-canvas-toggle-minimap"
          onClick={onToggleMinimap}
          aria-pressed={showMinimap}
          className={`rounded px-2 py-1 hover:bg-[var(--color-surface-2)] ${
            showMinimap ? 'text-[var(--color-accent)]' : 'text-[var(--color-text-dim)]'
          }`}
          title={showMinimap ? 'Mini haritayı gizle' : 'Mini haritayı göster'}
        >
          🗺 harita
        </button>
      </div>
    </Panel>
  )
}

const nodeTypes: NodeTypes = {
  agent: AgentNode,
  branch: BranchNode,
  parallel: ParallelNode,
  delay: DelayNode,
  transform: TransformNode,
  loop: LoopNode,
}

// Parallel-node edge colors so the two outgoing roles read at a glance: the
// fan-out edges (concurrent children) vs the single join edge (runs after all
// children finish; matches the parallel node's violet accent + join handle).
const FAN_EDGE_COLOR = '#0ea5e9' // sky — concurrent fan-out
const JOIN_EDGE_COLOR = '#7c3aed' // violet — join

// edgeColor returns the stroke color for an edge by its source handle, or
// undefined to use the default edge color (agent next / branch arms).
function edgeColor(sourceHandle: string | null | undefined): string | undefined {
  if (sourceHandle === 'fan') return FAN_EDGE_COLOR
  if (sourceHandle === 'join') return JOIN_EDGE_COLOR
  if (sourceHandle === 'body') return '#db2777' // loop body — pink (matches loop accent)
  if (sourceHandle === 'loop') return '#3b82f6' // loop exit — blue
  return undefined
}

// Built-in React Flow edge path styles the user can switch between.
export type EdgeStyle = 'default' | 'smoothstep' | 'step' | 'straight'

interface Props {
  agents: Agent[]
  nodes: FlowRFNode[]
  edges: Edge[]
  edgeStyle: EdgeStyle
  animated: boolean
  onNodesChange: (c: NodeChange<FlowRFNode>[]) => void
  onEdgesChange: (c: EdgeChange[]) => void
  setEdges: (updater: (e: Edge[]) => Edge[]) => void
  onSelect: (id: string | null) => void
  // Fired on a genuine node click (not a drag) — used to open the node editor
  // popup. React Flow suppresses onNodeClick when the pointer moved past the
  // drag threshold, so dragging to reposition/connect never triggers it.
  onNodeClick?: (id: string) => void
  // Read-only preview (template gallery): disable dragging, connecting and
  // selection so the graph can only be viewed, not edited.
  readOnly?: boolean
  // Re-layout the graph (parent recomputes node positions). Hidden if absent.
  onAutoLayout?: () => void
  // Per-node toolbar actions (make-start / duplicate / delete). Null = none.
  nodeActions?: NodeActions | null
  // Palette drag-and-drop: called with the dropped node type + the flow-space
  // position (already screen→flow converted). Absent = DnD disabled.
  onDropNode?: (type: FlowNodeType, pos: { x: number; y: number }) => void
}

// dataTransfer key for palette drag-and-drop of a new node.
export const FLOW_NODE_DND_MIME = 'application/tionswarm-flow-node'

// CanvasInner holds the actual <ReactFlow>. It lives inside ReactFlowProvider so
// it can use screenToFlowPosition to convert a drop point into graph space.
function CanvasInner({
  nodes,
  edges,
  edgeStyle,
  animated,
  onNodesChange,
  onEdgesChange,
  setEdges,
  onSelect,
  onNodeClick,
  readOnly = false,
  onAutoLayout,
  onDropNode,
}: Omit<Props, 'agents' | 'nodeActions'>) {
  const { screenToFlowPosition } = useReactFlow()
  // Mini-map show/hide (toggled from the in-canvas toolbar). Defaults on for wide
  // screens but OFF on narrow (< md) ones where it would crowd the canvas.
  const [showMinimap, setShowMinimap] = useState(
    () => typeof window === 'undefined' || window.innerWidth >= 768,
  )

  // Apply the chosen path style + animation + arrowhead to every edge for
  // display. These are cosmetic flow-level presentation hints; labels are kept.
  const styledEdges = useMemo(
    () =>
      edges.map((e) => {
        const color = edgeColor(e.sourceHandle)
        return {
          ...e,
          type: edgeStyle,
          animated,
          style: color ? { ...e.style, stroke: color } : e.style,
          markerEnd: {
            type: MarkerType.ArrowClosed,
            width: 18,
            height: 18,
            ...(color ? { color } : {}),
          },
        }
      }),
    [edges, edgeStyle, animated],
  )
  const onConnect = useCallback(
    (conn: Connection) => {
      setEdges((eds) => {
        const single = conn.sourceHandle !== 'fan' // fan-out allows many targets
        const pruned = single
          ? eds.filter((e) => !(e.source === conn.source && e.sourceHandle === conn.sourceHandle))
          : eds
        const label =
          conn.sourceHandle === 'join'
            ? 'join'
            : conn.sourceHandle?.startsWith('b')
              ? undefined // branch arm label comes from its condition (set on save/inspector)
              : undefined
        return addEdge({ ...conn, label }, pruned)
      })
    },
    [setEdges],
  )

  const onSelectionChange = useCallback(
    ({ nodes: sel }: OnSelectionChangeParams) => onSelect(sel[0]?.id ?? null),
    [onSelect],
  )

  const onDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    e.dataTransfer.dropEffect = 'move'
  }, [])

  const onDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault()
      const type = e.dataTransfer.getData(FLOW_NODE_DND_MIME) as FlowNodeType
      if (!type || !onDropNode) return
      const pos = screenToFlowPosition({ x: e.clientX, y: e.clientY })
      onDropNode(type, pos)
    },
    [screenToFlowPosition, onDropNode],
  )

  return (
    <ReactFlow
      data-testid="flow-canvas-root"
      nodes={nodes}
      edges={styledEdges}
      nodeTypes={nodeTypes}
      defaultEdgeOptions={{
        type: edgeStyle,
        animated,
        markerEnd: { type: MarkerType.ArrowClosed, width: 18, height: 18 },
      }}
      onNodesChange={onNodesChange}
      onEdgesChange={onEdgesChange}
      onConnect={readOnly ? undefined : onConnect}
      onSelectionChange={onSelectionChange}
      onNodeClick={readOnly || !onNodeClick ? undefined : (_, n) => onNodeClick(n.id)}
      onDrop={readOnly ? undefined : onDrop}
      onDragOver={readOnly ? undefined : onDragOver}
      // A few px of movement counts as a drag (not a click), so repositioning
      // a node never opens the editor popup and a plain click always does.
      nodeDragThreshold={4}
      nodesDraggable={!readOnly}
      nodesConnectable={!readOnly}
      elementsSelectable={!readOnly}
      fitView
      proOptions={{ hideAttribution: true }}
    >
      <Background />
      <Controls />
      <CanvasTools
        onAutoLayout={readOnly ? undefined : onAutoLayout}
        showMinimap={showMinimap}
        onToggleMinimap={() => setShowMinimap((v) => !v)}
      />
      {showMinimap && (
        <MiniMap
          pannable
          zoomable
          bgColor="#0b0e14"
          maskColor="rgba(0, 0, 0, 0.6)"
          nodeColor={(n) => chromeFor(n.type ?? 'agent').accent}
        />
      )}
    </ReactFlow>
  )
}

// FlowCanvas renders the interactive node graph. Connecting from a source
// handle replaces any existing edge from the same handle, so an agent's `next`
// and a branch arm stay single-target (parallel "fan" may have many).
export function FlowCanvas({ agents, nodeActions = null, ...rest }: Props) {
  return (
    <AgentsContext.Provider value={agents}>
      <NodeActionsContext.Provider value={nodeActions}>
        <ReactFlowProvider>
          <CanvasInner {...rest} />
        </ReactFlowProvider>
      </NodeActionsContext.Provider>
    </AgentsContext.Provider>
  )
}
