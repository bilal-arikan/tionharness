import { useMemo } from 'react'
import {
  ReactFlow,
  ReactFlowProvider,
  Background,
  Controls,
  MiniMap,
  type Edge,
  type NodeTypes,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import type { ViewRef } from '@/types'
import { ExplorerNode } from './ExplorerNode'
import type { ExplorerRFNode } from './explorerModel'

interface Props {
  nodes: ExplorerRFNode[]
  edges: Edge[]
  // A node click both selects it (side panel summary) and drills one layer in /
  // collapses it — the map's single interaction.
  onNodeClick: (ref: ViewRef) => void
}

// ExplorerGraph is the React Flow canvas for the drill-down map. Nodes are not
// draggable/connectable — position is derived from the expansion, not authored —
// so the canvas is pan/zoom + click only.
export function ExplorerGraph({ nodes, edges, onNodeClick }: Props) {
  const nodeTypes = useMemo<NodeTypes>(() => ({ explorer: ExplorerNode }), [])

  return (
    <ReactFlowProvider>
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={nodeTypes}
        onNodeClick={(_, n) => onNodeClick((n as ExplorerRFNode).data.ref)}
        nodesDraggable={false}
        nodesConnectable={false}
        elementsSelectable
        fitView
        fitViewOptions={{ padding: 0.3, maxZoom: 1 }}
        minZoom={0.2}
        proOptions={{ hideAttribution: true }}
      >
        <Background gap={18} color="var(--color-border)" />
        <Controls showInteractive={false} />
        <MiniMap pannable zoomable className="!bg-[var(--color-surface-2)]" />
      </ReactFlow>
    </ReactFlowProvider>
  )
}
