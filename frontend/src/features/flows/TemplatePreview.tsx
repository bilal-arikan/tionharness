import { useEffect } from 'react'
import { useNodesState, useEdgesState, type Edge } from '@xyflow/react'
import { graphToReactFlow, type FlowRFNode } from './flowGraph'
import type { Agent, FlowGraph } from '@/types'
import { FlowCanvas } from './FlowCanvas'

interface Props {
  graph: FlowGraph
  agents: Agent[]
}

// TemplatePreview renders a flow graph on a read-only canvas. It keeps local
// node/edge state (so React Flow can measure nodes and route edges) but the
// canvas is non-interactive — used by the template gallery to preview a
// template's structure before instantiating it into a real flow.
export function TemplatePreview({ graph, agents }: Props) {
  const [nodes, setNodes, onNodesChange] = useNodesState<FlowRFNode>([])
  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>([])

  useEffect(() => {
    const { nodes: rn, edges: re } = graphToReactFlow(graph)
    setNodes(rn)
    setEdges(re)
  }, [graph, setNodes, setEdges])

  return (
    <FlowCanvas
      agents={agents}
      nodes={nodes}
      edges={edges}
      edgeStyle={(graph.edgeStyle as never) || 'default'}
      animated={!!graph.animated}
      onNodesChange={onNodesChange}
      onEdgesChange={onEdgesChange}
      setEdges={setEdges}
      onSelect={() => {}}
      readOnly
    />
  )
}
