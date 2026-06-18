// Orchestration flows and their run history (Phase 7).
import type { Attachment, Flow, FlowGraph, FlowRun, Message } from '../types'
import { req, wsHeaders, errorFromResponse } from './client'

// One node lifecycle event streamed while a flow runs (mirrors
// orchestration.NodeEvent): phase "start" before a node runs, "done" after a
// success, "error" when the node fails (so a live spinner can stop and show why).
export interface FlowNodeEvent {
  phase: 'start' | 'done' | 'error'
  nodeId: string
  type: string
  title: string
  index: number
  output?: string
  error?: string
}

// Handlers invoked as a streamed flow run dispatches parsed SSE events.
export interface FlowStreamHandlers {
  attachments?: Attachment[]
  onMeta?: (m: { userMessage: Message }) => void
  onNode: (ev: FlowNodeEvent) => void
  onReply: (r: { replyMessage: Message }) => void
  onError: (err: string) => void
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

// streamRunFlow POSTs to the in-session SSE run-flow endpoint and dispatches
// parsed events (fetch streaming, since EventSource can't POST).
async function streamRunFlow(
  sessionId: string,
  flowId: string,
  input: string,
  handlers: FlowStreamHandlers,
  signal?: AbortSignal,
): Promise<void> {
  const res = await fetch(`/api/sessions/${sessionId}/run-flow-stream`, {
    method: 'POST',
    headers: wsHeaders(),
    body: JSON.stringify({ flowId, input, attachments: handlers.attachments ?? [] }),
    signal,
  })
  if (!res.ok || !res.body) {
    handlers.onError(await errorFromResponse(res))
    return
  }
  await pumpSSE(res, (event, data) => {
    switch (event) {
      case 'meta':
        handlers.onMeta?.(data as { userMessage: Message })
        break
      case 'node':
        handlers.onNode(data as FlowNodeEvent)
        break
      case 'reply':
        handlers.onReply(data as { replyMessage: Message })
        break
      case 'error':
        handlers.onError((data as { error: string }).error)
        break
    }
  })
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
  createFlow: (name: string, description = '', graph?: FlowGraph) =>
    req<Flow>('/api/flows', {
      method: 'POST',
      body: JSON.stringify({ name, description, graph }),
    }),
  updateFlow: (id: string, name: string, description: string, graph: FlowGraph) =>
    req<Flow>(`/api/flows/${id}`, {
      method: 'PUT',
      body: JSON.stringify({ name, description, graph }),
    }),
  deleteFlow: (id: string) =>
    req<{ result: string }>(`/api/flows/${id}`, { method: 'DELETE' }),
  // Run a flow standalone (FlowsPanel). The backend also records the run into
  // the flow's transcript session; we unwrap to the FlowRun for the panel.
  runFlow: (id: string, input: string) =>
    req<{ run: FlowRun; sessionId: string }>(`/api/flows/${id}/run`, {
      method: 'POST',
      body: JSON.stringify({ input }),
    }).then((r) => r.run),
  listFlowRuns: (flowId: string) =>
    req<FlowRun[]>(`/api/flow-runs?flowId=${encodeURIComponent(flowId)}`),
  // All flow runs across flows (newest first) — backend returns everything when
  // no flowId is given. Used by the FlowsPanel "Koşular" tab.
  listAllFlowRuns: () => req<FlowRun[]>('/api/flow-runs'),
  getFlowRun: (id: string) => req<FlowRun>(`/api/flow-runs/${id}`),
  // Run a flow inside a session over SSE, streaming each node's progress.
  runFlowStream: (
    sessionId: string,
    flowId: string,
    input: string,
    handlers: FlowStreamHandlers,
    signal?: AbortSignal,
  ): Promise<void> => streamRunFlow(sessionId, flowId, input, handlers, signal),
  // Run a flow standalone (FlowsPanel) over SSE, streaming node-by-node progress.
  runFlowStreamStandalone: (
    flowId: string,
    input: string,
    handlers: FlowRunStreamHandlers,
    signal?: AbortSignal,
  ): Promise<void> => streamRunFlowStandalone(flowId, input, handlers, signal),
}
