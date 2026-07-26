import type { Dispatch, SetStateAction } from 'react'
import type { Edge } from '@xyflow/react'
import { api } from '@/api'
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
  edges: Edge[]
  start: string
  edgeStyle: EdgeStyle
  animated: boolean
  accumulate: boolean
  input: string
  setRunning: Dispatch<SetStateAction<boolean>>
  // Runs tab: doRun surfaces the fresh run here (not painted on the editor canvas).
  runs: FlowRun[]
  setRuns: Dispatch<SetStateAction<FlowRun[]>>
  setSelectedRunId: Dispatch<SetStateAction<string | null>>
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
  edges,
  start,
  edgeStyle,
  animated,
  accumulate,
  input,
  setRunning,
  runs,
  setRuns,
  setSelectedRunId,
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
      graph.accumulate = accumulate
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

  // doRun starts the selected flow and shows its progress in the Koşular (Runs)
  // tab — NOT painted onto the editor canvas, so the flow-editing screen stays
  // exactly as the user left it. It switches to the Runs tab and auto-selects the
  // freshly started run (RunView then streams it live via the flow-node bus).
  const doRun = async () => {
    if (!selectedId) return
    setRunning(true)
    try {
      await saveFlow() // persist edits before running
      // Runs already listed for this flow — so we can pick the NEW one (not an old
      // run) as soon as it appears in the polled list.
      const priorIds = new Set(runs.filter((r) => r.flowId === selectedId).map((r) => r.id))
      setTab('runs')
      const refresh = () =>
        api
          .listAllFlowRuns()
          .then((rs) => {
            setRuns(rs)
            const fresh = rs.find((r) => r.flowId === selectedId && !priorIds.has(r.id))
            if (fresh) setSelectedRunId(fresh.id)
          })
          .catch(() => {})
      await api.runFlowStreamStandalone(selectedId, input, {
        // Each node event surfaces/advances the running run in the list; the run
        // viewer renders it live off the flow-node bus.
        onNode: () => refresh(),
        onReply: (r) => {
          setRuns((prev) => [r.run, ...prev.filter((x) => x.id !== r.run.id)])
          setSelectedRunId(r.run.id)
        },
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
