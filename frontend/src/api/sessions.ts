// Sessions: listing, titles, on-demand summaries, read state, disk/info and
// the per-session context meter.
import type { Session, Message, SessionInfo, SessionContext } from '../types'
import { req } from './client'

export const sessionApi = {
  // List sessions for an agent, or all sessions in the workspace when omitted.
  listSessions: (agentId?: string) =>
    req<Session[]>(
      agentId ? `/api/sessions?agentId=${encodeURIComponent(agentId)}` : '/api/sessions',
    ),
  // Session ids with a turn still streaming server-side. Queried after a reload
  // to restore the "thinking" indicator for detached turns still in flight.
  activeSessions: () =>
    req<{ sessionIds: string[] }>('/api/sessions/active').then((r) => r.sessionIds),
  createSession: (agentId = '', title = '') =>
    req<Session>('/api/sessions', {
      method: 'POST',
      body: JSON.stringify({ agentId, title }),
    }),
  listMessages: (sessionId: string) =>
    req<Message[]>(`/api/sessions/${sessionId}/messages`),
  // (Re)generate a session title — from its conversation, or an explicit source.
  generateSessionTitle: (sessionId: string, source?: string) =>
    req<{ id: string; title: string }>(`/api/sessions/${sessionId}/title`, {
      method: 'POST',
      body: JSON.stringify(source ? { source } : {}),
    }),
  // Set a session title verbatim (manual rename).
  setSessionTitle: (sessionId: string, title: string) =>
    req<{ id: string; title: string }>(`/api/sessions/${sessionId}/title`, {
      method: 'POST',
      body: JSON.stringify({ title }),
    }),
  // Set (or clear, when empty) the session's persistent goal ("north star").
  setSessionGoal: (sessionId: string, goal: string) =>
    req<{ id: string; goal: string }>(`/api/sessions/${sessionId}/goal`, {
      method: 'PUT',
      body: JSON.stringify({ goal }),
    }),
  // On-demand summary/listing posted as an assistant message in the session.
  // kind: 'memory' | 'board' | 'flows' | 'tools'. Returns the new message.
  summarizeSession: (sessionId: string, kind: string) =>
    req<{ userMessage: Message; replyMessage: Message }>(`/api/sessions/${sessionId}/summary`, {
      method: 'POST',
      body: JSON.stringify({ kind }),
    }),
  // Run a flow and record its result as a turn in this session (user input +
  // assistant transcript). Powers triggering flows from the chat "/" menu.
  runFlowInSession: (sessionId: string, flowId: string, input: string) =>
    req<{ userMessage: Message; replyMessage: Message }>(`/api/sessions/${sessionId}/run-flow`, {
      method: 'POST',
      body: JSON.stringify({ flowId, input }),
    }),
  // Clear a session's unread flag.
  markSessionRead: (sessionId: string) =>
    req<{ id: string }>(`/api/sessions/${sessionId}/read`, { method: 'POST' }),
  // Delete a session and its on-disk folder.
  deleteSession: (sessionId: string) =>
    req<{ deleted: string }>(`/api/sessions/${sessionId}`, { method: 'DELETE' }),
  // Delete a single message from a session (prune a mistaken/test message).
  deleteMessage: (sessionId: string, messageId: string) =>
    req<{ deleted: string }>(`/api/sessions/${sessionId}/messages/${messageId}`, {
      method: 'DELETE',
    }),
  // Absolute folder holding the session's JSONL file.
  sessionPath: (sessionId: string) =>
    req<{ path: string }>(`/api/sessions/${sessionId}/path`),
  // Rich session detail: disk footprint, context composition, participating agents.
  sessionInfo: (sessionId: string) =>
    req<SessionInfo>(`/api/sessions/${sessionId}/info`),
  // Open the session's folder in the OS file manager (local desktop).
  revealSession: (sessionId: string) =>
    req<{ path: string }>(`/api/sessions/${sessionId}/reveal`, { method: 'POST' }),

  sessionContext: (sessionId: string) =>
    req<SessionContext>(`/api/sessions/${sessionId}/context`),
}
