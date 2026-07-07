import type { Dispatch, SetStateAction } from 'react'
import type { Edge } from '@xyflow/react'
import { api } from '@/api'
import type { FlowNodeEvent } from '@/api/flows'
import type { EdgeStyle } from './FlowCanvas'
import type { FlowTemplate } from './flowTemplates'
import { reactFlowToGraph, type FlowRFNode } from './flowGraph'
import type { Agent, Flow, FlowRun } from '@/types'
import type { MultiSelect } from '@/shared/hooks/useMultiSelect'
import type { FlowsTab } from './flowsPanelShared'

// Dependencies the flow-level actions (CRUD / save / run) need from FlowsPanel.
interface FlowActionsDeps {
  agents: Agent[]
  selectedId: string | null
  setSelectedId: Dispatch<SetStateAction<string | null>>
  setFlows: Dispatch<SetStateAction<Flow[]>>
  loadFlows: () => void
  selectFlow: (f: Flow) => void
  setTab: Dispatch<SetStateAction<FlowsTab>>
  name: string
  setEmoji: Dispatch<SetStateAction<string>>
  nodes: FlowRFNode[]
  setNodes: Dispatch<SetStateAction<FlowRFNode[]>>
  edges: Edge[]
  start: string
  edgeStyle: EdgeStyle
  animated: boolean
  input: string
  setRunning: Dispatch<SetStateAction<boolean>>
  setRun: Dispatch<SetStateAction<FlowRun | null>>
  setLiveNodes: Dispatch<SetStateAction<FlowNodeEvent[]>>
  sel: MultiSelect
  onError: (msg: string) => void
}

// createFlowActions builds the flow-level actions (create / instantiate /
// save / emoji / delete / bulk ops / run) over FlowsPanel's state. Re-created
// each render by FlowsPanel, exactly like the original inline definitions.
export function createFlowActions({
  agents,
  selectedId,
  setSelectedId,
  setFlows,
  loadFlows,
  selectFlow,
  setTab,
  name,
  setEmoji,
  nodes,
  setNodes,
  edges,
  start,
  edgeStyle,
  animated,
  input,
  setRunning,
  setRun,
  setLiveNodes,
  sel,
  onError,
}: FlowActionsDeps) {
  const createFlow = async () => {
    const n = prompt('Akış adı:')
    if (!n) return
    try {
      const f = await api.createFlow(n)
      setFlows((prev) => [f, ...prev])
      selectFlow(f)
    } catch (e) {
      onError((e as Error).message)
    }
  }

  // instantiateTemplate creates a new editable flow from a template's graph,
  // then switches to it for editing. Template agent nodes are unassigned; the
  // backend requires every agent node to have an agentId, so we seed each with
  // the first available agent as a placeholder for the user to reassign.
  const instantiateTemplate = async (t: FlowTemplate) => {
    const defaultAgent = agents[0]?.id
    if (!defaultAgent) {
      onError('Şablondan akış oluşturmak için önce en az bir ajan oluşturun.')
      return
    }
    const graph = {
      ...t.graph,
      nodes: t.graph.nodes.map((n) =>
        n.type === 'agent' && !n.agentId ? { ...n, agentId: defaultAgent } : n,
      ),
    }
    try {
      const f = await api.createFlow(t.name, graph)
      setFlows((prev) => [f, ...prev])
      setTab('flows')
      selectFlow(f)
    } catch (e) {
      onError((e as Error).message)
    }
  }

  const saveFlow = async () => {
    if (!selectedId) return
    try {
      const graph = reactFlowToGraph(nodes, edges, start)
      graph.edgeStyle = edgeStyle
      graph.animated = animated
      const f = await api.updateFlow(selectedId, name, graph)
      setFlows((prev) => prev.map((x) => (x.id === f.id ? f : x)))
      onError('') // clear
    } catch (e) {
      onError((e as Error).message)
    }
  }

  // changeEmoji persists the selected flow's emoji immediately (independent of
  // the Save button) and syncs the local list so the glyph updates everywhere.
  const changeEmoji = async (next: string) => {
    setEmoji(next) // optimistic
    if (!selectedId) return
    try {
      await api.setFlowEmoji(selectedId, next)
      setFlows((prev) => prev.map((x) => (x.id === selectedId ? { ...x, emoji: next } : x)))
    } catch (e) {
      onError((e as Error).message)
    }
  }

  const removeFlow = async (f: Flow) => {
    if (!confirm(`"${f.name}" akışı silinsin mi?`)) return
    try {
      await api.deleteFlow(f.id)
      if (selectedId === f.id) setSelectedId(null)
      loadFlows()
    } catch (e) {
      onError((e as Error).message)
    }
  }

  const bulkRun = async () => {
    const ids = [...sel.selected]
    if (ids.length === 0) return
    sel.clear()
    try {
      await Promise.all(ids.map((id) => api.runFlow(id, '')))
    } catch (e) {
      onError((e as Error).message)
    }
  }
  const bulkDelete = async () => {
    const ids = [...sel.selected]
    if (ids.length === 0) return
    if (!confirm(`${ids.length} akış silinsin mi?`)) return
    if (selectedId && sel.selected.has(selectedId)) setSelectedId(null)
    sel.clear()
    try {
      await Promise.all(ids.map((id) => api.deleteFlow(id)))
      loadFlows()
    } catch (e) {
      onError((e as Error).message)
    }
  }

  // setNodeStatus paints a node's live run state (running glow / done ring /
  // error ring).
  const setNodeStatus = (nodeId: string, status: 'running' | 'done' | 'error' | undefined) => {
    setNodes((prev) =>
      prev.map((rn) => (rn.id === nodeId ? { ...rn, data: { ...rn.data, status } } : rn)),
    )
  }

  const doRun = async () => {
    if (!selectedId) return
    setRunning(true)
    setRun(null)
    setLiveNodes([])
    setNodes((prev) => prev.map((rn) => ({ ...rn, data: { ...rn.data, status: undefined } })))
    try {
      await saveFlow() // persist edits before running
      await api.runFlowStreamStandalone(selectedId, input, {
        onNode: (ev) => {
          const status = ev.phase === 'start' ? 'running' : ev.phase === 'error' ? 'error' : 'done'
          setNodeStatus(ev.nodeId, status)
          setLiveNodes((prev) => {
            if (ev.phase === 'start') return [...prev, ev]
            // done/error: replace the pending entry for this node (still running).
            const i = prev.findIndex((n) => n.nodeId === ev.nodeId && n.output === undefined && n.error === undefined)
            if (i < 0) return [...prev, ev]
            const next = [...prev]
            next[i] = ev
            return next
          })
        },
        onReply: (r) => setRun(r.run),
        onError: (e) => onError(e),
      })
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setRunning(false)
    }
  }

  return {
    createFlow,
    instantiateTemplate,
    saveFlow,
    changeEmoji,
    removeFlow,
    bulkRun,
    bulkDelete,
    doRun,
  }
}
