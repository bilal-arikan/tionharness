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
  createdAt: number
  updatedAt: number
}

// WorkdirInfo is the composer folder badge's view of a session's working dir.
export interface WorkdirInfo {
  dir: string // session override ("" = none)
  effective: string // override or workspace default
  exists: boolean
  isGitRepo: boolean
  branch: string
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
