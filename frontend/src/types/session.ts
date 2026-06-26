// Sessions plus the rich detail/context model behind the session info panel.

export interface Session {
  id: string
  agentId: string
  // Broad category of what produced the transcript: chat | task | flow |
  // schedule. Drives the executions feed's kind badge.
  kind: string
  // Links the session to the entity that owns it (a task or flow id); empty for
  // plain chat and agent-keyed kinds (schedule).
  sourceId?: string
  title: string
  messageCount: number
  state: string
  // True when an agent reply landed while this session wasn't open.
  unread?: boolean
  // Working directory (cwd) override for the agent's file/shell tools. Empty =
  // workspace default. Set from the composer folder badge.
  workingDir?: string
  // Context-reset lineage: when this session was born from a /handoff, it points
  // back to the session it continues; handoffArtifactId is the handoff written
  // into THIS session at the moment work was reset out of it. Both empty for an
  // ordinary session.
  parentSessionId?: string
  handoffArtifactId?: string
  createdAt: number
  updatedAt: number
}

// SearchHit is one message matched by cross-session full-text search
// (GET /api/sessions/search). Carries sessionId + messageId so a result can
// deep-link to the originating turn.
export interface SearchHit {
  sessionId: string
  sessionTitle: string
  messageId: string
  role: string
  agentId?: string
  snippet: string
  score: number
  createdAt: number
}

// WorkdirInfo is the composer folder badge's view of a session's working dir.
export interface WorkdirInfo {
  dir: string // session override ("" = none)
  effective: string // override or workspace default
  exists: boolean
  isGitRepo: boolean
  branch: string
}

// ProgressTodo is one persisted checklist item (durable todo_write entry).
export interface ProgressTodo {
  content: string
  status: 'pending' | 'in_progress' | 'completed'
  category?: string
  steps?: string[]
}

// ProgressLogEntry is one line of the rolling progress journal.
export interface ProgressLogEntry {
  ts: number
  sessionId?: string
  note: string
}

// ProgressRecord is the decoded persistent-progress file (todos + log).
export interface ProgressRecord {
  version: number
  updatedAt: number
  sessionId?: string
  agentId?: string
  todos: ProgressTodo[]
  log?: ProgressLogEntry[]
}

// SessionProgress is the read-only viewer's view of a session's progress file.
export interface SessionProgress {
  path: string
  exists: boolean
  record: ProgressRecord | null
}

// BrowseEntry is one selectable directory in the folder picker.
export interface BrowseEntry {
  name: string
  path: string
}

// BrowseResp is the folder picker's view of one directory level.
export interface BrowseResp {
  path: string // listed dir ("" = drive/root list)
  parent: string // parent dir ("" at a root)
  entries: BrowseEntry[]
}

// GitInfo is the Project panel's view of a path's git state.
export interface GitInfo {
  path: string
  exists: boolean
  isGitRepo: boolean
  branch: string
  remote: string
  userName: string
  userEmail: string
}

// Execution is one row of the unified executions feed (GET /api/executions): a
// session-backed transcript produced by any path (chat/task/flow/schedule),
// enriched with the agent name, a live-running flag and a last-run status.
export interface Execution {
  sessionId: string
  kind: string
  sourceId?: string
  title: string
  agentId: string
  agentName: string
  messageCount: number
  unread: boolean
  running: boolean
  lastStatus?: string
  createdAt: number
  updatedAt: number
}

export interface SessionContext {
  contextTokens: number
  hasSummary: boolean
  summaryMsgCount: number
  messageCount: number
}

// The exact next-turn context a session's agent would be sent (debug preview):
// composed system + dynamic suffix, the message transcript, and the tool catalog,
// each with a token estimate. Backed by GET /api/sessions/{id}/context-preview.
export interface SessionContextPreview {
  agentName: string
  multiAgent: boolean
  system: string
  systemTokens: number
  dynamic: string
  dynamicTokens: number
  messages: { role: string; text: string; author?: string; self?: boolean }[]
  messageTokens: number
  tools: { name: string; description: string; inputSchema?: unknown }[]
  toolTokens: number
  totalTokens: number
  cache: CachePreview
}

// Which segments of the next request are served from a warm prompt cache vs sent
// fresh. anthropic caches the Tools + System prefix (when ExtendedPromptCache is
// on); claude-cli --resume keeps System + the first cachedMsgCount messages warm
// server-side and sends only the newest delta. mode "none" = nothing cached.
export interface CachePreview {
  mode: 'anthropic' | 'claude-resume' | 'none'
  note: string
  systemCached: boolean
  dynamicCached: boolean
  toolsCached: boolean
  cachedMsgCount: number
}

// A labelled bucket of the live context window (summary or a message role).
export interface ContextFiller {
  label: string
  role: string
  tokens: number
  count: number
}

// An agent that took part in a session, with per-agent turn/token counts.
export interface SessionAgentStat {
  agentId: string
  name: string
  avatar?: string
  color?: string
  turns: number
  tokens: number
  isOwner: boolean
  disabled: boolean
}

// Rich session detail (GET /api/sessions/{id}/info) behind the detail panel:
// on-disk footprint, context composition and participating agents.
export interface SessionInfo {
  id: string
  title: string
  kind: string
  state: string
  agentId: string
  agentName: string
  messageCount: number
  unread: boolean
  // Persistent session objective ("north star") injected into every turn's
  // context. Empty when none is set.
  goal: string
  // True when the goal is marked done: it stays visible but is no longer
  // injected into context.
  goalDone: boolean
  // Context-reset lineage: the session this one continues (born from /handoff)
  // and the handoff artifact written into this session at reset.
  parentSessionId?: string
  handoffArtifactId?: string
  createdAt: number
  updatedAt: number

  path: string
  sizeBytes: number
  fileCount: number

  contextTokens: number
  contextWindow: number
  hasSummary: boolean
  summaryMsgCount: number
  summaryTokens: number

  fillers: ContextFiller[]
  agents: SessionAgentStat[]
}
