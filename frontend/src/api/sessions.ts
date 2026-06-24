// Sessions: listing, titles, on-demand summaries, read state, disk/info and
// the per-session context meter.
import type {
  Session,
  Message,
  SessionInfo,
  SessionContext,
  SessionContextPreview,
  SearchHit,
  WorkdirInfo,
  BrowseResp,
  GitInfo,
  SessionUsageDetail,
} from '../types'
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
  // Spawn a new independent session and run the agent's turn in the background
  // (fire-and-forget). Returns the new session id, which surfaces live in the
  // executions feed. modelOverride swaps only the model (provider unchanged).
  spawnSession: (agentId: string, prompt: string, modelOverride = '') =>
    req<{ sessionId: string; agentName: string }>('/api/sessions/spawn', {
      method: 'POST',
      body: JSON.stringify({ agentId, prompt, modelOverride }),
    }),
  listMessages: (sessionId: string) =>
    req<Message[]>(`/api/sessions/${sessionId}/messages`),
  // Full-text search the workspace's message history. role: 'user' | 'assistant'
  // | 'all'; exclude skips a session id (e.g. the current one). Each hit carries
  // sessionId + messageId for deep-linking to the matched turn.
  searchMessages: (
    q: string,
    opts: { limit?: number; role?: string; exclude?: string } = {},
  ) => {
    const p = new URLSearchParams({ q })
    if (opts.limit) p.set('limit', String(opts.limit))
    if (opts.role && opts.role !== 'all') p.set('role', opts.role)
    if (opts.exclude) p.set('exclude', opts.exclude)
    return req<SearchHit[]>(`/api/sessions/search?${p.toString()}`)
  },
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
  // done marks it achieved (kept visible, no longer injected into context).
  setSessionGoal: (sessionId: string, goal: string, done = false) =>
    req<{ id: string; goal: string; goalDone: boolean }>(`/api/sessions/${sessionId}/goal`, {
      method: 'PUT',
      body: JSON.stringify({ goal, done }),
    }),
  // Rebind the session to a different agent (the chat agent dropdown). Every
  // following turn is answered by this agent.
  setSessionAgent: (sessionId: string, agentId: string) =>
    req<{ id: string; agentId: string }>(`/api/sessions/${sessionId}/agent`, {
      method: 'PUT',
      body: JSON.stringify({ agentId }),
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

  // Per-session lifetime spend + savings (cost, per-origin/model breakdown,
  // cache savings, tool-output compaction bytes). The session-scoped analog of
  // agentUsage — this conversation's own cost, not the agent's whole-day total.
  sessionUsageDetail: (sessionId: string) =>
    req<SessionUsageDetail>(`/api/sessions/${sessionId}/usage-detail`),

  // Debug: preview the exact next-turn context (system + dynamic + transcript +
  // tools) the session's agent would be sent. Optional sample "next" user message.
  sessionContextPreview: (sessionId: string, message?: string) =>
    req<SessionContextPreview>(
      `/api/sessions/${sessionId}/context-preview${
        message ? `?message=${encodeURIComponent(message)}` : ''
      }`,
    ),

  // Working directory (cwd) for the agent's file/shell tools.
  getWorkdir: (sessionId: string) =>
    req<WorkdirInfo>(`/api/sessions/${sessionId}/workdir`),
  // Set (or clear, when dir is empty) the session's working directory.
  setWorkdir: (sessionId: string, dir: string) =>
    req<WorkdirInfo>(`/api/sessions/${sessionId}/workdir`, {
      method: 'PUT',
      body: JSON.stringify({ dir }),
    }),
  // List subdirectories of a path for the folder picker (empty path = roots).
  browseDirs: (path: string) =>
    req<BrowseResp>(`/api/fs/browse?path=${encodeURIComponent(path)}`),

  // Git state of a project path (repo?, branch, remote, identity).
  gitInfo: (path: string) =>
    req<GitInfo>(`/api/fs/gitinfo?path=${encodeURIComponent(path)}`),
  // Initialise a git repo (default branch "main") in an existing directory.
  gitInit: (path: string) =>
    req<GitInfo>('/api/git/init', { method: 'POST', body: JSON.stringify({ path }) }),
  // Apply repo-local git settings (origin remote URL + user.name/email).
  gitConfig: (path: string, cfg: { remote?: string; userName?: string; userEmail?: string }) =>
    req<GitInfo>('/api/git/config', {
      method: 'POST',
      body: JSON.stringify({ path, ...cfg }),
    }),
}
