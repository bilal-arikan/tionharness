// Sessions: listing, titles, on-demand summaries, read state, disk/info and
// the per-session context meter.
import type {
  Session,
  Message,
  SessionInfo,
  WorkerInfo,
  CoordinatorTree,
  CoordinatorAncestor,
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
  SessionChangeStep,
} from '@/types'
import { req } from './client'

// SessionPage is the paged listing envelope returned by listSessions — the API
// twin of the agent tool's pageResult (TSK68). A parameter-less call still
// receives the legacy unwrapped array on the wire and is normalized to this
// shape client-side, so every caller can rely on items/total/hasMore.
export interface SessionPage {
  items: Session[]
  total: number
  offset: number
  limit: number
  hasMore: boolean
  // chipKey → how many sessions the query's non-chip scope holds for that chip.
  // Counted before the chips filter, so an unticked badge still reports what it
  // hides. Absent on the legacy unwrapped response.
  chipCounts?: Record<string, number>
}

// asSessionPage normalizes either wire shape (unwrapped legacy array or paged
// envelope) to the paged form.
export function asSessionPage(r: Session[] | SessionPage): SessionPage {
  return Array.isArray(r)
    ? { items: r, total: r.length, offset: 0, limit: r.length, hasMore: false }
    : r
}

export interface SessionListParams {
  agentId?: string
  // Bounded exact lookup used to recover deep-linked/drafted sessions outside
  // the current filtered page.
  ids?: string[]
  limit?: number
  offset?: number
  // sort in the tool-layer format, e.g. "updated_desc" | "created_asc" | "title_asc".
  sort?: string
  kind?: string
  state?: string
  // Comma-separated sidebar chip keys. The server applies the same chip
  // predicate BEFORE paging, so total/hasMore describe the rows the sidebar can
  // actually show — without it a page of mixed kinds can be almost entirely
  // filtered out client-side and "load more" looks broken. An empty string is a
  // real selection (nothing ticked) and matches nothing.
  chips?: string
}

export const sessionApi = {
  // List sessions for an agent, or all sessions in the workspace when omitted.
  // Paging params (limit/offset/sort/kind/state) switch the server to the
  // {items,total,offset,limit,hasMore} envelope (TSK68); a parameter-less call
  // still receives the legacy unwrapped array and is normalized here so every
  // caller can rely on the paged shape.
  listSessions: (params?: SessionListParams): Promise<SessionPage> => {
    const p = new URLSearchParams()
    if (params?.agentId) p.set('agentId', params.agentId)
    if (params?.ids?.length) p.set('ids', params.ids.join(','))
    if (params?.limit !== undefined) p.set('limit', String(params.limit))
    if (params?.offset !== undefined) p.set('offset', String(params.offset))
    if (params?.sort) p.set('sort', params.sort)
    if (params?.kind) p.set('kind', params.kind)
    if (params?.state) p.set('state', params.state)
    if (params?.chips !== undefined) p.set('chips', params.chips)
    const qs = p.toString()
    return req<Session[] | SessionPage>(qs ? `/api/sessions?${qs}` : '/api/sessions').then(
      asSessionPage,
    )
  },
  getSessionsByIds: (ids: string[]): Promise<Session[]> => {
    if (ids.length === 0) return Promise.resolve([])
    return sessionApi.listSessions({ ids, limit: ids.length }).then((page) => page.items)
  },
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
  // (fire-and-forget). Capacity may queue it before a session id exists.
  // modelOverride swaps only the model (provider unchanged).
  spawnSession: (agentId: string, prompt: string, modelOverride = '') =>
    req<{ sessionId: string; agentName: string; queued: boolean; queuePosition?: number }>(
      '/api/sessions/spawn',
      {
        method: 'POST',
        body: JSON.stringify({ agentId, prompt, modelOverride }),
      },
    ),
  listMessages: (sessionId: string) => req<Message[]>(`/api/sessions/${sessionId}/messages`),
  // One turn's activity trace, UNTRIMMED. listMessages ships tool payloads cut
  // to a server-side cap (marked with the step's `*Truncated` flags) so opening
  // a long session stays cheap; this refetches the full trace for a single turn
  // when the user asks to see it. Returns the raw JSON string persisted on the
  // message — feed it to parseSteps().
  getMessageSteps: (sessionId: string, messageId: string) =>
    req<{ steps: string }>(
      `/api/sessions/${encodeURIComponent(sessionId)}/messages/${encodeURIComponent(messageId)}/steps`,
    ).then((r) => r.steps),
  // Every file mutation in the session, oldest first, with UNTRIMMED patches.
  // Backs the "all changes" popup's session tab: assembling it from the
  // transcript would ship every Read/Grep/Bash payload too, just to find the
  // edits, so the server filters them out before sending.
  getSessionChanges: (sessionId: string) =>
    req<SessionChangeStep[]>(`/api/sessions/${encodeURIComponent(sessionId)}/changes`),
  // Full-text search the workspace's message history. role: 'user' | 'assistant'
  // | 'all'; exclude skips a session id (e.g. the current one). Each hit carries
  // sessionId + messageId for deep-linking to the matched turn.
  searchMessages: (q: string, opts: { limit?: number; role?: string; exclude?: string } = {}) => {
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
  // Turn the session's COORDINATOR MODE on ('coordinator') or off (''). The field
  // is still named `role` on the wire, but it no longer touches the session's
  // lineage: a worker toggled on here becomes a mid-level node of its tree rather
  // than being cut loose from its parent. Rejected (400) while the session still
  // has running workers.
  setSessionRole: (sessionId: string, role: string) =>
    req<{ id: string; role: string }>(`/api/sessions/${sessionId}/role`, {
      method: 'PUT',
      body: JSON.stringify({ role }),
    }),
  // Select (or clear) the coordinator recipe/workflow (M5) for a session. Pass a
  // coordinator-workflow skill slug to apply it, or '' to clear. Rejects an
  // invalid slug (not a coordinator-workflow, or unknown pattern).
  setSessionWorkflow: (sessionId: string, workflow: string) =>
    req<{ id: string; workflow: string; maxTurns: number }>(`/api/sessions/${sessionId}/workflow`, {
      method: 'PUT',
      body: JSON.stringify({ workflow }),
    }),
  // Resume a coordinator hard-halted by the phantom-spawn stall guard: clears the
  // halt + nudge streak and kicks one fresh coordinator turn. The "Devam ettir" CTA.
  resumeCoordinator: (sessionId: string) =>
    req<{ id: string; resumed: boolean }>(`/api/sessions/${sessionId}/coordinator/resume`, {
      method: 'POST',
    }),
  // List the workers spawned under a coordinator session (for the coordination panel).
  listWorkers: (sessionId: string) =>
    req<{ workers: WorkerInfo[] }>(`/api/sessions/${sessionId}/workers`),
  // "Buradan çatalla" (Rota F5): spawn a worker under a coordinator session
  // from the canvas, as spawn_worker would from inside its turn.
  spawnWorker: (
    sessionId: string,
    body: { agent: string; task: string; coordinator?: boolean; workflow?: string; cwd?: string },
  ) =>
    req<{ sessionId: string; agentName: string; queued: boolean; queuePosition: number }>(
      `/api/sessions/${sessionId}/workers`,
      { method: 'POST', body: JSON.stringify(body) },
    ),
  // The whole coordinator TREE a session belongs to, breadth-first from its root.
  // Callable with ANY member's id (root, mid-level node, or leaf) — the server
  // normalizes to the root — so the panel can pass whatever session is open.
  getCoordinatorTree: (sessionId: string) =>
    req<CoordinatorTree>(`/api/sessions/${sessionId}/coordinator-tree`),
  // The upward breadcrumb from a worker: its tree root first, its direct
  // coordinator last. Empty for a root or an ordinary session.
  getCoordinatorAncestors: (sessionId: string) =>
    req<{ ancestors: CoordinatorAncestor[] }>(`/api/sessions/${sessionId}/coordinator-ancestors`),
  // On-demand summary/listing posted as an assistant message in the session.
  // kind: 'board' | 'flows' | 'tools'. Returns the new message.
  summarizeSession: (sessionId: string, kind: string) =>
    req<{ userMessage: Message; replyMessage: Message }>(`/api/sessions/${sessionId}/summary`, {
      method: 'POST',
      body: JSON.stringify({ kind }),
    }),
  // Context reset (/handoff): write a handoff artifact for this session and spawn
  // a FRESH session to continue the work in a clean window. Returns the new
  // session id (the UI switches to it) and the user "/handoff" bubble. When the
  // session is a coordinator with running workers the reset is refused: no
  // newSessionId, blocked=true, and replyMessage carries the in-thread notice.
  handoffSession: (sessionId: string) =>
    req<{
      userMessage: Message
      newSessionId?: string
      agentName?: string
      artifactId?: string
      blocked?: boolean
      replyMessage?: Message
    }>(`/api/sessions/${sessionId}/handoff`, { method: 'POST' }),
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
  sessionPath: (sessionId: string) => req<{ path: string }>(`/api/sessions/${sessionId}/path`),
  // Rich session detail: disk footprint, context composition, participating agents.
  sessionInfo: (sessionId: string) => req<SessionInfo>(`/api/sessions/${sessionId}/info`),
  // Recycle the session's warm (persistent-pool) claude-cli process so the next
  // turn cold-restarts fresh. Conversation untouched. Returns how many were dropped.
  dropSessionCliProcess: (sessionId: string) =>
    req<{ dropped: number }>(`/api/sessions/${sessionId}/cli-process`, { method: 'DELETE' }),

  sessionContext: (sessionId: string) => req<SessionContext>(`/api/sessions/${sessionId}/context`),

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
    req<TurnDebug>(`/api/sessions/${sessionId}/turn-debug?turn=${encodeURIComponent(turnId)}`),

  // Debug: preview the exact next-turn context (system + dynamic + transcript +
  // tools) the session's agent would be sent. Optional sample "next" user message.
  // compact=true simulates this turn's budgeted compaction (read-only, no summary
  // generated/persisted) so the message array matches what the model receives.
  // accurate=true additionally asks the provider's REAL tokenizer to count the
  // composed request server-side (/v1/messages/count_tokens; anthropic only) —
  // returned as accurateTokens so heuristic drift is visible.
  sessionContextPreview: (
    sessionId: string,
    message?: string,
    compact = false,
    accurate = false,
  ) => {
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
  getWorkdir: (sessionId: string) => req<WorkdirInfo>(`/api/sessions/${sessionId}/workdir`),
  // Set (or clear, when dir is empty) the session's working directory.
  setWorkdir: (sessionId: string, dir: string) =>
    req<WorkdirInfo>(`/api/sessions/${sessionId}/workdir`, {
      method: 'PUT',
      body: JSON.stringify({ dir }),
    }),
  // List subdirectories of a path for the folder picker (empty path = roots).
  browseDirs: (path: string) => req<BrowseResp>(`/api/fs/browse?path=${encodeURIComponent(path)}`),

  // Git state of a project path (repo?, branch, remote, identity).
  gitInfo: (path: string) => req<GitInfo>(`/api/fs/gitinfo?path=${encodeURIComponent(path)}`),
  // Initialise a git repo (default branch "main") in a directory. createDir opts
  // into laying the folder out first when it does not exist yet.
  gitInit: (path: string, createDir = false) =>
    req<GitInfo>('/api/git/init', { method: 'POST', body: JSON.stringify({ path, createDir }) }),
  // Apply repo-local git settings (origin remote URL + user.name/email).
  gitConfig: (path: string, cfg: { remote?: string; userName?: string; userEmail?: string }) =>
    req<GitInfo>('/api/git/config', {
      method: 'POST',
      body: JSON.stringify({ path, ...cfg }),
    }),
}
