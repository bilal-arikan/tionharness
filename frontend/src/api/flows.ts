// Orchestration flows and their run history (Phase 7).
import type { Flow, FlowGraph, FlowRun, TurnStep } from '@/types'
import { req, wsHeaders, errorFromResponse } from './client'

// One node lifecycle event streamed while a flow runs (mirrors
// orchestration.NodeEvent): phase "start" before a node runs, "done" after a
// success, "error" when the node fails (so a live spinner can stop and show why).
export interface FlowNodeEvent {
  phase: 'start' | 'done' | 'error' | 'waiting' | 'progress'
  nodeId: string
  type: string
  title: string
  index: number
  output?: string
  error?: string
}

// Handlers for a standalone flow run (FlowsPanel) streamed over SSE: node
// progress, then the finished run plus the transcript session it produced.
export interface FlowRunStreamHandlers {
  onNode: (ev: FlowNodeEvent) => void
  onReply: (r: { run: FlowRun; sessionId: string }) => void
  onError: (err: string) => void
}

// dispatchSSE parses a raw SSE frame ("event:"/"data:" lines) and routes it.
function dispatchSSE(frame: string, route: (event: string, data: unknown) => void) {
  let event = 'message'
  const dataLines: string[] = []
  for (const line of frame.split('\n')) {
    if (line.startsWith('event:')) event = line.slice(6).trim()
    else if (line.startsWith('data:')) dataLines.push(line.slice(5).trim())
  }
  if (dataLines.length === 0) return
  let data: unknown
  try {
    data = JSON.parse(dataLines.join('\n'))
  } catch {
    return
  }
  route(event, data)
}

// pumpSSE reads a streamed response body frame-by-frame until it ends.
async function pumpSSE(res: Response, route: (event: string, data: unknown) => void) {
  const reader = res.body!.getReader()
  const decoder = new TextDecoder()
  let buf = ''
  for (;;) {
    const { done, value } = await reader.read()
    if (done) break
    buf += decoder.decode(value, { stream: true })
    let idx: number
    while ((idx = buf.indexOf('\n\n')) >= 0) {
      const frame = buf.slice(0, idx)
      buf = buf.slice(idx + 2)
      if (frame.trim()) dispatchSSE(frame, route)
    }
  }
}

// streamRunFlowStandalone POSTs to the standalone SSE run endpoint (FlowsPanel),
// streaming node progress and ending with the finished run + its session id.
async function streamRunFlowStandalone(
  flowId: string,
  input: string,
  handlers: FlowRunStreamHandlers,
  signal?: AbortSignal,
): Promise<void> {
  const res = await fetch(`/api/flows/${flowId}/run-stream`, {
    method: 'POST',
    headers: wsHeaders(),
    body: JSON.stringify({ input }),
    signal,
  })
  if (!res.ok || !res.body) {
    handlers.onError(await errorFromResponse(res))
    return
  }
  await pumpSSE(res, (event, data) => {
    switch (event) {
      case 'node':
        handlers.onNode(data as FlowNodeEvent)
        break
      case 'reply':
        handlers.onReply(data as { run: FlowRun; sessionId: string })
        break
      case 'error':
        handlers.onError((data as { error: string }).error)
        break
    }
  })
}

export const flowApi = {
  listFlows: () => req<Flow[]>('/api/flows'),
  createFlow: (name: string, graph?: FlowGraph, emoji?: string) =>
    req<Flow>('/api/flows', {
      method: 'POST',
      body: JSON.stringify({ name, graph, emoji }),
    }),
  updateFlow: (id: string, name: string, graph: FlowGraph) =>
    req<Flow>(`/api/flows/${id}`, {
      method: 'PUT',
      body: JSON.stringify({ name, graph }),
    }),
  deleteFlow: (id: string) => req<{ result: string }>(`/api/flows/${id}`, { method: 'DELETE' }),
  // Removes a FINISHED run and its whole tree (409 while any member is live).
  deleteFlowRun: (id: string) =>
    req<{ deleted: string[] }>(`/api/flow-runs/${id}`, { method: 'DELETE' }),
  // Replace a flow's free-form tags (organizational).
  setFlowTags: (id: string, tags: string[]) =>
    req<{ id: string; tags: string[] }>(`/api/flows/${id}/tags`, {
      method: 'PUT',
      body: JSON.stringify({ tags }),
    }),
  // Replace a flow's cosmetic emoji (persisted independently of name/graph).
  setFlowEmoji: (id: string, emoji: string) =>
    req<{ id: string; emoji: string }>(`/api/flows/${id}/emoji`, {
      method: 'PUT',
      body: JSON.stringify({ emoji }),
    }),
  // Absolute path of the flow's on-disk JSON file (copy-to-clipboard).
  flowPath: (id: string) => req<{ path: string }>(`/api/flows/${id}/path`),
  // Open the flow's folder in the OS file manager (local desktop).
  // Run a flow standalone (FlowsPanel). The backend also records the run into
  // the flow's transcript session; we unwrap to the FlowRun for the panel.
  runFlow: (id: string, input: string) =>
    req<{ run: FlowRun; sessionId: string }>(`/api/flows/${id}/run`, {
      method: 'POST',
      body: JSON.stringify({ input }),
    }).then((r) => r.run),
  // Fetch a single flow by id (chat "Akış olarak gör" resolves a flow session
  // back to its real graph). Rejects (404) if the flow was deleted.
  getFlow: (id: string) => req<Flow>(`/api/flows/${encodeURIComponent(id)}`),
  listFlowRuns: (flowId: string) =>
    req<FlowRun[]>(`/api/flow-runs?flowId=${encodeURIComponent(flowId)}`),
  // All flow runs across flows (newest first) — backend returns everything when
  // no flowId is given. Used by the FlowsPanel "Koşular" tab. Pass rootOnly to
  // leave out the subflow/spawn children of composed flows, so one run of a
  // composed flow is one row; the children stay reachable via flowRunTree.
  listAllFlowRuns: (rootOnly = false) =>
    req<FlowRun[]>(`/api/flow-runs${rootOnly ? '?rootOnly=true' : ''}`),
  getFlowRun: (id: string) => req<FlowRun>(`/api/flow-runs/${id}`),
  // Every run in one composed flow's tree, breadth-first (parent before its
  // children). Accepts ANY member id, not just the root — the backend normalises
  // to the root. Also the resync path when the live event stream drops.
  flowRunTree: (id: string) => req<FlowRun[]>(`/api/flow-runs/${encodeURIComponent(id)}/tree`),
  // One agent node's captured tool/thinking steps for a run, read from the
  // per-node sidecar. Returns [] for nodes with no steps (or pre-capture runs).
  flowRunNodeSteps: (runId: string, nodeId: string) =>
    req<TurnStep[]>(
      `/api/flow-runs/${encodeURIComponent(runId)}/nodes/${encodeURIComponent(nodeId)}/steps`,
    ),
  // Deliver input to a run suspended at an await-input node and resume it. Only a
  // "waiting" run accepts input; a non-waiting/already-resumed run returns 409.
  resumeFlowRun: (id: string, input: string) =>
    req<{ run: FlowRun }>(`/api/flow-runs/${id}/input`, {
      method: 'POST',
      body: JSON.stringify({ input }),
    }).then((r) => r.run),
  // Run a flow standalone (FlowsPanel) over SSE, streaming node-by-node progress.
  runFlowStreamStandalone: (
    flowId: string,
    input: string,
    handlers: FlowRunStreamHandlers,
    signal?: AbortSignal,
  ): Promise<void> => streamRunFlowStandalone(flowId, input, handlers, signal),
}
