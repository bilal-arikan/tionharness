import { useCallback } from 'react'
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

interface Props {
  agents: Agent[]
  nodes: FlowRFNode[]
  edges: Edge[]
  onNodesChange: (c: NodeChange<FlowRFNode>[]) => void
  onEdgesChange: (c: EdgeChange[]) => void
  setEdges: (updater: (e: Edge[]) => Edge[]) => void
  onSelect: (id: string | null) => void
}

// FlowCanvas renders the interactive node graph. Connecting from a source
// handle replaces any existing edge from the same handle, so an agent's `next`
// and a branch arm stay single-target (parallel "fan" may have many).
export function FlowCanvas({
  agents,
  nodes,
  edges,
  onNodesChange,
  onEdgesChange,
  setEdges,
  onSelect,
}: Props) {
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
          edges={edges}
          nodeTypes={nodeTypes}
          onNodesChange={onNodesChange}
          onEdgesChange={onEdgesChange}
          onConnect={onConnect}
          onSelectionChange={onSelectionChange}
          fitView
          proOptions={{ hideAttribution: true }}
        >
          <Background />
          <Controls />
          <MiniMap
            pannable
            zoomable
            nodeColor={(n) => chromeFor(n.type ?? 'agent').accent}
          />
        </ReactFlow>
      </ReactFlowProvider>
    </AgentsContext.Provider>
  )
}
