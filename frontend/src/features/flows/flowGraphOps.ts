import type { Dispatch, SetStateAction } from 'react'
import type { Edge } from '@xyflow/react'
import {
  reactFlowToGraph,
  autoLayout,
  blankNode,
  nextNodeId,
  type FlowRFNode,
} from './flowGraph'
import type { Agent, FlowNode, FlowNodeType } from '@/types'

// Dependencies the canvas node operations need from FlowsPanel's editor state.
interface FlowGraphOpsDeps {
  agents: Agent[]
  nodes: FlowRFNode[]
  setNodes: Dispatch<SetStateAction<FlowRFNode[]>>
  edges: Edge[]
  setEdges: Dispatch<SetStateAction<Edge[]>>
  start: string
  setStart: Dispatch<SetStateAction<string>>
  selectedNodeId: string | null
  setSelectedNodeId: Dispatch<SetStateAction<string | null>>
}

// createFlowGraphOps builds the node-level canvas operations (add / patch /
// start / delete / duplicate / auto-arrange) over the editor state. Re-created
// each render by FlowsPanel, exactly like the original inline definitions.
export function createFlowGraphOps({
  agents,
  nodes,
  setNodes,
  edges,
  setEdges,
  start,
  setStart,
  selectedNodeId,
  setSelectedNodeId,
}: FlowGraphOpsDeps) {
  // addNodeAt appends a blank node of the given type at a specific canvas
  // position and makes it the start node if none is set yet.
  const addNodeAt = (type: FlowNodeType, pos: { x: number; y: number }) => {
    const existing = nodes.map((n) => n.data.node)
    const id = nextNodeId(existing)
    const node = blankNode(id, type, agents[0]?.id ?? '')
    const rf: FlowRFNode = {
      id,
      type,
      position: pos,
      data: { node, isStart: !start },
    }
    setNodes((prev) => [...prev, rf])
    if (!start) setStart(id)
  }

  // addNode (click on the palette) drops the new node near the canvas origin,
  // cascaded so successive adds don't stack exactly on top of each other.
  const addNode = (type: FlowNodeType) => {
    const offset = nodes.length * 30
    addNodeAt(type, { x: 80 + offset, y: 80 + offset })
  }

  // patchSelected updates the selected node's intrinsic fields. A type change or
  // a shrunk branch list prunes now-invalid outgoing edges so the graph stays
  // consistent on save.
  const patchSelected = (patch: Partial<FlowNode>) => {
    if (!selectedNodeId) return
    let prune = false
    setNodes((prev) =>
      prev.map((rn) => {
        if (rn.id !== selectedNodeId) return rn
        const before = rn.data.node
        const merged = { ...before, ...patch }
        if (patch.type && patch.type !== before.type) prune = true
        if (
          patch.branches &&
          patch.branches.length < (before.branches?.length ?? 0)
        )
          prune = true
        return { ...rn, type: merged.type, data: { ...rn.data, node: merged } }
      }),
    )
    if (prune) {
      setEdges((eds) => eds.filter((e) => e.source !== selectedNodeId))
    }
  }

  // Node actions are id-based so both the inspector (on the selected node) and
  // each node's NodeToolbar can invoke them.
  const makeStartNode = (id: string) => {
    setStart(id)
    setNodes((prev) => prev.map((rn) => ({ ...rn, data: { ...rn.data, isStart: rn.id === id } })))
  }

  const deleteNode = (id: string) => {
    setNodes((prev) => prev.filter((rn) => rn.id !== id))
    setEdges((eds) => eds.filter((e) => e.source !== id && e.target !== id))
    if (start === id) setStart('')
    if (selectedNodeId === id) setSelectedNodeId(null)
  }

  // duplicateNode clones a node (new id, offset position, not start) without its
  // connections — the copy starts unwired.
  const duplicateNode = (id: string) => {
    setNodes((prev) => {
      const src = prev.find((rn) => rn.id === id)
      if (!src) return prev
      const newId = nextNodeId(prev.map((rn) => rn.data.node))
      const copy: FlowRFNode = {
        id: newId,
        type: src.type,
        position: { x: src.position.x + 40, y: src.position.y + 40 },
        data: { node: { ...src.data.node, id: newId }, isStart: false },
      }
      return [...prev, copy]
    })
  }

  const makeStart = () => {
    if (selectedNodeId) makeStartNode(selectedNodeId)
  }
  const deleteSelected = () => {
    if (selectedNodeId) deleteNode(selectedNodeId)
  }
  const duplicateSelected = () => {
    if (selectedNodeId) duplicateNode(selectedNodeId)
  }

  // autoArrange re-lays-out the graph with the layered grid algorithm and
  // applies the computed positions to the canvas nodes.
  const autoArrange = () => {
    const graph = reactFlowToGraph(nodes, edges, start)
    const pos = autoLayout(graph)
    setNodes((prev) =>
      prev.map((rn) => (pos[rn.id] ? { ...rn, position: { x: pos[rn.id].x, y: pos[rn.id].y } } : rn)),
    )
  }

  return {
    addNodeAt,
    addNode,
    patchSelected,
    makeStartNode,
    deleteNode,
    duplicateNode,
    makeStart,
    deleteSelected,
    duplicateSelected,
    autoArrange,
  }
}
