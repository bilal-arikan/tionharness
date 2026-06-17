// Orchestration flows and their run history (Phase 7).
import type { Attachment, Flow, FlowGraph, FlowRun, Message } from '../types'
import { req, wsHeaders, errorFromResponse } from './client'

// One node lifecycle event streamed while a flow runs (mirrors
// orchestration.NodeEvent): phase "start" before a node runs, "done" after.
export interface FlowNodeEvent {
  phase: 'start' | 'done'
  nodeId: string
  type: string
  title: string
  index: number
  output?: string
}

// Handlers invoked as a streamed flow run dispatches parsed SSE events.
export interface FlowStreamHandlers {
  attachments?: Attachment[]
  onMeta?: (m: { userMessage: Message }) => void
  onNode: (ev: FlowNodeEvent) => void
  onReply: (r: { replyMessage: Message }) => void
  onError: (err: string) => void
}

// streamRunFlow POSTs to the SSE run-flow endpoint and dispatches parsed events
// (fetch streaming, since EventSource can't POST). Resolves when the stream ends.
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
  const reader = res.body.getReader()
  const decoder = new TextDecoder()
  let buf = ''

  const dispatch = (frame: string) => {
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
  }

  for (;;) {
    const { done, value } = await reader.read()
    if (done) break
    buf += decoder.decode(value, { stream: true })
    let idx: number
    while ((idx = buf.indexOf('\n\n')) >= 0) {
      const frame = buf.slice(0, idx)
      buf = buf.slice(idx + 2)
      if (frame.trim()) dispatch(frame)
    }
  }
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
  runFlow: (id: string, input: string) =>
    req<FlowRun>(`/api/flows/${id}/run`, {
      method: 'POST',
      body: JSON.stringify({ input }),
    }),
  listFlowRuns: (flowId: string) =>
    req<FlowRun[]>(`/api/flow-runs?flowId=${encodeURIComponent(flowId)}`),
  getFlowRun: (id: string) => req<FlowRun>(`/api/flow-runs/${id}`),
  // Run a flow inside a session over SSE, streaming each node's progress.
  runFlowStream: (
    sessionId: string,
    flowId: string,
    input: string,
    handlers: FlowStreamHandlers,
    signal?: AbortSignal,
  ): Promise<void> => streamRunFlow(sessionId, flowId, input, handlers, signal),
}
