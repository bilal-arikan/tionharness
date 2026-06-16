// Chat endpoints: the blocking turn, the SSE streaming turn (with its frame
// parser) and the in-flight control channel.
import type { Attachment, Message, ChatResponse, TurnStep } from '../types'
import { req, wsHeaders, errorFromResponse } from './client'

// Handlers invoked as the streaming turn dispatches parsed SSE events. Also
// carries the turn's attachments (data, not a callback) sent in the request body.
export interface ChatStreamHandlers {
  // Files / pasted text uploaded for this turn (already on the server).
  attachments?: Attachment[]
  onMeta?: (m: { userMessage: Message; runId: string }) => void
  onAgentStart?: (a: { agentId: string; index: number }) => void
  onStep: (step: TurnStep) => void
  onReply: (r: { replyMessage: Message }) => void
  onDone: (d: { sessionTitle?: string }) => void
  onError: (err: string) => void
}

// streamChat POSTs to the SSE endpoint and dispatches parsed events. Uses fetch
// streaming (EventSource can't POST). Resolves when the stream ends.
async function streamChat(
  sessionId: string,
  message: string,
  agentIds: string[],
  handlers: ChatStreamHandlers,
  signal?: AbortSignal,
  thinkingLevel?: string,
  permissionMode?: string,
): Promise<void> {
  const res = await fetch('/api/chat/stream', {
    method: 'POST',
    headers: wsHeaders(),
    body: JSON.stringify({
      sessionId,
      message,
      agentIds,
      thinkingLevel,
      permissionMode,
      attachments: handlers.attachments ?? [],
    }),
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
        handlers.onMeta?.(data as { userMessage: Message; runId: string })
        break
      case 'agent':
        handlers.onAgentStart?.(data as { agentId: string; index: number })
        break
      case 'step':
        handlers.onStep(data as TurnStep)
        break
      case 'reply':
        handlers.onReply(data as { replyMessage: Message })
        break
      case 'done':
        handlers.onDone(data as { sessionTitle?: string })
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
    // SSE frames are separated by a blank line (\n\n).
    while ((idx = buf.indexOf('\n\n')) >= 0) {
      const frame = buf.slice(0, idx)
      buf = buf.slice(idx + 2)
      if (frame.trim()) dispatch(frame)
    }
  }
}

export const chatApi = {
  chat: (sessionId: string, message: string) =>
    req<ChatResponse>('/api/chat', {
      method: 'POST',
      body: JSON.stringify({ sessionId, message }),
    }),

  // Streaming chat: receive each activity step as it happens over SSE.
  chatStream: (
    sessionId: string,
    message: string,
    agentIds: string[],
    handlers: ChatStreamHandlers,
    signal?: AbortSignal,
    thinkingLevel?: string,
    permissionMode?: string,
  ): Promise<void> => streamChat(sessionId, message, agentIds, handlers, signal, thinkingLevel, permissionMode),

  // Control an in-flight streaming turn: stop (cancel), steer (live guidance) or
  // answer (reply to a blocked ask_user prompt).
  chatControl: (runId: string, action: 'stop' | 'steer' | 'answer', text?: string) =>
    req<{ result: string }>('/api/chat/control', {
      method: 'POST',
      body: JSON.stringify({ runId, action, text }),
    }),
}
