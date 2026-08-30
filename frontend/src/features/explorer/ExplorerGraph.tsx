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
import type { ViewHandle, ViewRef } from '@/types'
import { ExplorerNode } from './ExplorerNode'
import type { ExplorerRFNode } from './explorerModel'

interface Props {
  nodes: ExplorerRFNode[]
  edges: Edge[]
  onNodeClick: (ref: ViewRef) => void
  onNodeDoubleClick: (ref: ViewRef) => void
  onOverflowClick: (side: 'parents' | 'children', handles: ViewHandle[]) => void
}

// Positions come from the pure focus model; canvas owns only pan/zoom and input.
export function ExplorerGraph({
  nodes,
  edges,
  onNodeClick,
  onNodeDoubleClick,
  onOverflowClick,
}: Props) {
  const nodeTypes = useMemo<NodeTypes>(() => ({ explorer: ExplorerNode }), [])

  return (
    <ReactFlowProvider>
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={nodeTypes}
        onNodeClick={(_, n) => {
          const data = (n as ExplorerRFNode).data
          if (data.overflow) onOverflowClick(data.overflow.side, data.overflow.handles)
          else onNodeClick(data.ref)
        }}
        onNodeDoubleClick={(_, n) => {
          const data = (n as ExplorerRFNode).data
          if (!data.overflow) onNodeDoubleClick(data.ref)
        }}
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
