// Sessions: listing, titles, on-demand summaries, read state, disk/info and
// the per-session context meter.
import type {
  Session,
  Message,
  SessionInfo,
  WorkerInfo,
  SessionContext,
  SessionContextPreview,
  SearchHit,
  WorkdirInfo,
  BrowseResp,
  GitInfo,
  SessionUsageDetail,
  SessionProgress,
  SessionDebugSummary,
  SessionDebugEvent,
  TurnDebug,
  InflightSnapshot,
} from '@/types'
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
  // The session's in-progress streaming snapshot (partial reply — agent, text and
  // trace so far), or null when no turn is streaming. Fetched after a mid-turn
  // reload to restore the in-progress assistant bubble instead of losing its
  // steps/agent until the turn finishes.
  getInflight: (sessionId: string) =>
    req<InflightSnapshot | null>(`/api/sessions/${encodeURIComponent(sessionId)}/inflight`),
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
  // Set the session's lifecycle state ("active" | "archived"). Archiving drops it
  // from the active sidebar list but never deletes it; "active" restores it.
  setSessionState: (sessionId: string, state: 'active' | 'archived') =>
    req<{ id: string; state: string }>(`/api/sessions/${sessionId}/state`, {
      method: 'PUT',
      body: JSON.stringify({ state }),
    }),
  // Pin/unpin the session to the top of the sidebar list.
  setSessionPinned: (sessionId: string, pinned: boolean) =>
    req<{ id: string; pinned: boolean }>(`/api/sessions/${sessionId}/pin`, {
      method: 'PUT',
      body: JSON.stringify({ pinned }),
    }),
  // Replace the session's free-form tags (shared with agents; also drive automations).
  setSessionTags: (sessionId: string, tags: string[]) =>
    req<{ id: string; tags: string[] }>(`/api/sessions/${sessionId}/tags`, {
      method: 'PUT',
      body: JSON.stringify({ tags }),
    }),
  // Rate an assistant message (👍/👎 + optional note). rating: +1 | -1 | 0 (clear).
  setMessageFeedback: (sessionId: string, messageId: string, rating: number, note = '') =>
    req<{ id: string; rating: number; note: string }>(
      `/api/sessions/${sessionId}/messages/${messageId}/feedback`,
      { method: 'PUT', body: JSON.stringify({ rating, note }) },
    ),
  // Rebind the session to a different agent (the chat agent dropdown). Every
  // following turn is answered by this agent.
  setSessionAgent: (sessionId: string, agentId: string) =>
    req<{ id: string; agentId: string }>(`/api/sessions/${sessionId}/agent`, {
      method: 'PUT',
      body: JSON.stringify({ agentId }),
    }),
  // Set the session's coordinator role (M2). role: 'coordinator' to enable
  // coordinator mode (coordinator prompt + spawn_worker/... tools), '' to revert.
  setSessionRole: (sessionId: string, role: string) =>
    req<{ id: string; role: string }>(`/api/sessions/${sessionId}/role`, {
      method: 'PUT',
      body: JSON.stringify({ role }),
    }),
  // List the workers spawned under a coordinator session (for the coordination panel).
  listWorkers: (sessionId: string) =>
    req<{ workers: WorkerInfo[] }>(`/api/sessions/${sessionId}/workers`),
  // On-demand summary/listing posted as an assistant message in the session.
  // kind: 'board' | 'flows' | 'tools'. Returns the new message.
  summarizeSession: (sessionId: string, kind: string) =>
    req<{ userMessage: Message; replyMessage: Message }>(`/api/sessions/${sessionId}/summary`, {
      method: 'POST',
      body: JSON.stringify({ kind }),
    }),
  // Context reset (/handoff): write a handoff artifact for this session and spawn
  // a FRESH session to continue the work in a clean window. Returns the new
  // session id (the UI switches to it) and the user "/handoff" bubble.
  handoffSession: (sessionId: string) =>
    req<{ userMessage: Message; newSessionId: string; agentName: string; artifactId: string }>(
      `/api/sessions/${sessionId}/handoff`,
      { method: 'POST' },
    ),
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
  // Rewind the conversation to a checkpoint: remove the given message and every
  // message after it (conversation-only — file changes are NOT reverted).
  rewindSession: (sessionId: string, messageId: string) =>
    req<{ removed: number }>(`/api/sessions/${sessionId}/rewind`, {
      method: 'POST',
      body: JSON.stringify({ messageId }),
    }),
  // Absolute folder holding the session's JSONL file.
  sessionPath: (sessionId: string) =>
    req<{ path: string }>(`/api/sessions/${sessionId}/path`),
  // Rich session detail: disk footprint, context composition, participating agents.
  sessionInfo: (sessionId: string) =>
    req<SessionInfo>(`/api/sessions/${sessionId}/info`),
  // Recycle the session's warm (persistent-pool) claude-cli process so the next
  // turn cold-restarts fresh. Conversation untouched. Returns how many were dropped.
  dropSessionCliProcess: (sessionId: string) =>
    req<{ dropped: number }>(`/api/sessions/${sessionId}/cli-process`, { method: 'DELETE' }),
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

  // Persistent progress (durable todo_write checklist + rolling log) for the
  // read-only viewer card. Resolved from the session's working dir / store fallback.
  sessionProgress: (sessionId: string) =>
    req<SessionProgress>(`/api/sessions/${sessionId}/progress`),

  // Per-session debug journal aggregate (turn timings, token spend by model,
  // per-tool latency/size/errors, compaction/recovery counts) for the Debug tab.
  sessionDebugSummary: (sessionId: string) =>
    req<{ summary: SessionDebugSummary }>(`/api/sessions/${sessionId}/debug`).then(
      (r) => r.summary,
    ),
  // Raw per-session debug events (newest last), optionally filtered by type.
  sessionDebugEvents: (sessionId: string, type = '', limit = 200) => {
    const p = new URLSearchParams({ summary: '0', limit: String(limit) })
    if (type) p.set('type', type)
    return req<{ events: SessionDebugEvent[] }>(
      `/api/sessions/${sessionId}/debug?${p.toString()}`,
    ).then((r) => r.events ?? [])
  },

  // Per-MESSAGE debug rollup for one assistant reply (token spend, latency, cost,
  // per-tool breakdown), correlated by the reply message id. Backs the chat
  // message debug button.
  sessionTurnDebug: (sessionId: string, turnId: string) =>
    req<TurnDebug>(
      `/api/sessions/${sessionId}/turn-debug?turn=${encodeURIComponent(turnId)}`,
    ),

  // Debug: preview the exact next-turn context (system + dynamic + transcript +
  // tools) the session's agent would be sent. Optional sample "next" user message.
  // compact=true simulates this turn's budgeted compaction (read-only, no summary
  // generated/persisted) so the message array matches what the model receives.
  // accurate=true additionally asks the provider's REAL tokenizer to count the
  // composed request server-side (/v1/messages/count_tokens; anthropic only) —
  // returned as accurateTokens so heuristic drift is visible.
  sessionContextPreview: (sessionId: string, message?: string, compact = false, accurate = false) => {
    const p = new URLSearchParams()
    if (message) p.set('message', message)
    if (compact) p.set('compact', '1')
    if (accurate) p.set('accurate', '1')
    const q = p.toString()
    return req<SessionContextPreview>(
      `/api/sessions/${sessionId}/context-preview${q ? `?${q}` : ''}`,
    )
  },

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
