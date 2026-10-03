// Chat endpoints: queued turns, side chat and in-flight control.
import type { Attachment, BtwResponse } from '@/types'
import { req } from './client'

export const chatApi = {
  // Side chat ("btw"): ask a one-shot question against the session's context
  // WITHOUT writing it into the history. The agent gets no tools, and neither the
  // question nor the answer becomes a session message — so a long conversation's
  // token cost does not grow. Answerable while the main turn is still streaming.
  btw: (sessionId: string, question: string, agentId?: string) =>
    req<BtwResponse>('/api/chat/btw', {
      method: 'POST',
      body: JSON.stringify({ sessionId, question, agentId }),
    }),

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

  // Convert a waiting message into live guidance for the turn already running.
  // Atomic server-side: on any refusal ("unsupported", a full steer buffer, ...)
  // the message stays queued, so it is never lost and never runs twice.
  steerQueued: (sessionId: string, clientMsgId: string) =>
    req<{ result: string }>(`/api/sessions/${sessionId}/queue/${clientMsgId}/steer`, {
      method: 'POST',
    }),

  // Stop or steer a session's in-flight turn WITHOUT a runId (the queue runs turns
  // server-side, so control is session-scoped now).
  sessionControl: (sessionId: string, action: 'stop' | 'steer', text?: string) =>
    req<{ result: string }>(`/api/sessions/${sessionId}/control`, {
      method: 'POST',
      body: JSON.stringify({ action, text }),
    }),

  // Atomic interrupt: stop the in-flight turn AND claim the session's next turn
  // slot in ONE server-side step. Doing it as stop-then-send left the slot free
  // between the two round trips, so another queued message or an autonomous turn
  // (worker notification, self-wake) could take it and the "send now" message ran
  // after the very turn it was meant to cut in front of.
  // The message becomes a normal queued turn, so it carries the same per-turn
  // settings a normal send does — omitting them would silently downgrade it.
  interruptSession: (
    sessionId: string,
    body: {
      text: string
      clientMsgId: string
      agentIds: string[]
      thinkingLevel: string
      permissionMode: string
      attachments: Attachment[]
    },
  ) =>
    req<{ result: string; queued: boolean; stopped: boolean }>(
      `/api/sessions/${sessionId}/control`,
      {
        method: 'POST',
        body: JSON.stringify({ action: 'interrupt', ...body }),
      },
    ),

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
