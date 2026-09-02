// Session event stream client — the frontend half of the server-authoritative
// per-session hub (internal/sessionhub, _Docs/58-QUEUE-SENKRON.md). Every window
// watching a session opens ONE of these and renders the transcript live from it,
// whoever started the turn. It replaces the old owner-streams-its-own-SSE +
// non-owner-polls-inflight split.
//
// The transport (cursor, epoch, gap detection, reset, reconnect backoff, server
// clock) is the shared hub loop in hubStream.ts; this module only names the
// session endpoint and the session-specific event kinds.
import { subscribeHubStream } from './hubStream'
import type { HubEvent, HubStreamHandlers } from './hubStream'

export type { HubEvent } from './hubStream'

// A stable id for THIS browser window/tab, minted once. Used to tag outbound
// signals (typing) so the window can ignore its own echo on the shared hub.
export const windowClientId: string = (() => {
  const c = (globalThis as { crypto?: { randomUUID?: () => string } }).crypto
  if (c?.randomUUID) return c.randomUUID()
  return `win-${Date.now()}-${Math.random().toString(36).slice(2)}`
})()

export type SessionStreamHandlers = HubStreamHandlers

// Kinds mirror internal/sessionhub constants.
export const HubKind = {
  UserMessage: 'user_message',
  Step: 'step',
  Reply: 'reply',
  AgentStart: 'agent_start',
  InteractionOpen: 'interaction_open',
  InteractionResolved: 'interaction_resolved',
  SessionUpdate: 'session_update',
  TurnDone: 'turn_done',
  TurnError: 'turn_error',
  QueueUpdate: 'queue_update',
  Presence: 'presence',
  Delta: 'delta',
  ToolDelta: 'tool_delta',
  Tombstone: 'tombstone',
  Typing: 'typing',
} as const

// subscribeSessionStream opens the stream and keeps it alive across reconnects.
// Returns an unsubscribe function that stops the loop and aborts the fetch.
export function subscribeSessionStream(
  sessionId: string,
  handlers: SessionStreamHandlers,
): () => void {
  return subscribeHubStream(`/api/sessions/${sessionId}/stream`, handlers)
}

// Re-exported for callers that narrow on the event shape without importing the
// transport module.
export type { HubEvent as SessionHubEvent }
