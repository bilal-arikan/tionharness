// Chat endpoints: the blocking turn, the SSE streaming turn (with its frame
// parser) and the in-flight control channel.
import type { Attachment, Message, ChatResponse, TurnStep } from '@/types'
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
  // err is the human message; replyMessage is the server-persisted error message
  // (an assistant turn carrying an 'error' step) when the failure occurred after
  // the turn began — absent for pre-flight/transport failures.
  onError: (err: string, replyMessage?: Message) => void
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
      case 'error': {
        const e = data as { error: string; replyMessage?: Message }
        handlers.onError(e.error, e.replyMessage)
        break
      }
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

  // Send-queue (Faz 3): enqueue a user turn. Durable + idempotent on clientMsgId
  // (dedupes double-submits/retries). Returns immediately; the turn runs
  // server-side and streams to every window over the session hub.
  enqueueMessage: (
    sessionId: string,
    body: {
      message: string
      agentIds?: string[]
      attachments?: Attachment[]
      thinkingLevel?: string
      permissionMode?: string
      clientMsgId?: string
    },
  ) =>
    req<{ queued: boolean; clientMsgId: string }>(`/api/sessions/${sessionId}/messages`, {
      method: 'POST',
      body: JSON.stringify(body),
    }),

  // Cancel a not-yet-dispatched queued message (the in-flight turn is unaffected —
  // use control:"stop" for that).
  cancelQueued: (sessionId: string, clientMsgId: string) =>
    req<{ removed: boolean }>(`/api/sessions/${sessionId}/queue/${clientMsgId}`, {
      method: 'DELETE',
    }),

  // Drop every waiting message from a session's queue.
  clearQueue: (sessionId: string) =>
    req<{ cleared: number }>(`/api/sessions/${sessionId}/queue`, { method: 'DELETE' }),

  // Promote a waiting message so it dispatches next ("send next").
  moveQueuedFront: (sessionId: string, clientMsgId: string) =>
    req<{ moved: boolean }>(`/api/sessions/${sessionId}/queue/${clientMsgId}/front`, {
      method: 'POST',
    }),

  // Stop or steer a session's in-flight turn WITHOUT a runId (the queue runs turns
  // server-side, so control is session-scoped now).
  sessionControl: (sessionId: string, action: 'stop' | 'steer', text?: string) =>
    req<{ result: string }>(`/api/sessions/${sessionId}/control`, {
      method: 'POST',
      body: JSON.stringify({ action, text }),
    }),

  // Broadcast a cross-window "user is typing" signal (ephemeral). clientId lets
  // the sending window ignore its own echo.
  setTyping: (sessionId: string, active: boolean, clientId: string) =>
    req<{ result: string }>(`/api/sessions/${sessionId}/typing`, {
      method: 'POST',
      body: JSON.stringify({ active, clientId }),
    }),

  // Answer a resolve-once interaction (ask_user / permission / plan) via CAS. The
  // first window to answer wins (200); a concurrent second answer gets 409 so the
  // UI can simply close the already-resolved card. See _Docs/58-QUEUE-SENKRON.md.
  answerInteraction: (
    sessionId: string,
    interactionId: string,
    body: { answer?: string; answersJson?: string; clientId?: string },
  ) =>
    req<{ result: string }>(`/api/sessions/${sessionId}/interactions/${interactionId}/answer`, {
      method: 'POST',
      body: JSON.stringify(body),
    }),

  // Disarm a pending one-shot self-wake (schedule_wake) for a session — the user
  // pressed "Durdur" on the waiting banner before the wake fired.
  cancelWake: (sessionId: string) =>
    req<{ result: string; cancelled: number }>('/api/chat/wake/cancel', {
      method: 'POST',
      body: JSON.stringify({ sessionId }),
    }),
}
