import { useCallback, useMemo } from 'react'
import {
  ReactFlow,
  ReactFlowProvider,
  Background,
  Controls,
  MiniMap,
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
import type { Agent } from '../../types'
import type { FlowRFNode } from '../../lib/flowGraph'
import { AgentsContext, chromeFor } from './nodeStyles'
import { AgentNode } from './AgentNode'
import { BranchNode } from './BranchNode'
import { ParallelNode } from './ParallelNode'

const nodeTypes: NodeTypes = {
  agent: AgentNode,
  branch: BranchNode,
  parallel: ParallelNode,
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
  // Read-only preview (template gallery): disable dragging, connecting and
  // selection so the graph can only be viewed, not edited.
  readOnly?: boolean
}

// FlowCanvas renders the interactive node graph. Connecting from a source
// handle replaces any existing edge from the same handle, so an agent's `next`
// and a branch arm stay single-target (parallel "fan" may have many).
export function FlowCanvas({
  agents,
  nodes,
  edges,
  edgeStyle,
  animated,
  onNodesChange,
  onEdgesChange,
  setEdges,
  onSelect,
  readOnly = false,
}: Props) {
  // Apply the chosen path style + animation to every edge for display. These
  // are cosmetic flow-level presentation hints; the labels/handles are kept.
  const styledEdges = useMemo(
    () => edges.map((e) => ({ ...e, type: edgeStyle, animated })),
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

  return (
    <AgentsContext.Provider value={agents}>
      <ReactFlowProvider>
        <ReactFlow
          nodes={nodes}
          edges={styledEdges}
          nodeTypes={nodeTypes}
          defaultEdgeOptions={{ type: edgeStyle, animated }}
          onNodesChange={onNodesChange}
          onEdgesChange={onEdgesChange}
          onConnect={readOnly ? undefined : onConnect}
          onSelectionChange={onSelectionChange}
          nodesDraggable={!readOnly}
          nodesConnectable={!readOnly}
          elementsSelectable={!readOnly}
          fitView
          proOptions={{ hideAttribution: true }}
        >
          <Background />
          <Controls />
          <MiniMap
            pannable
            zoomable
            bgColor="#0b0e14"
            maskColor="rgba(0, 0, 0, 0.6)"
            nodeColor={(n) => chromeFor(n.type ?? 'agent').accent}
          />
        </ReactFlow>
      </ReactFlowProvider>
    </AgentsContext.Provider>
  )
}
