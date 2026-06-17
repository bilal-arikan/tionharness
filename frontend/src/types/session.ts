// Sessions plus the rich detail/context model behind the session info panel.

export interface Session {
  id: string
  agentId: string
  // Broad category of what produced the transcript: chat | task | flow |
  // schedule | heartbeat. Drives the executions feed's kind badge.
  kind: string
  // Links the session to the entity that owns it (a task or flow id); empty for
  // plain chat and agent-keyed kinds (schedule/heartbeat).
  sourceId?: string
  title: string
  messageCount: number
  state: string
  // True when an agent reply landed while this session wasn't open.
  unread?: boolean
  createdAt: number
  updatedAt: number
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
