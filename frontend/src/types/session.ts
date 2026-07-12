// Sessions plus the rich detail/context model behind the session info panel.

export interface Session {
  id: string
  // The DEFAULT responder: the agent that answers when a turn carries no explicit
  // per-turn routing. `participants` is the full roster the composer can route to.
  agentId: string
  // Roster of agent ids taking part in this thread, beyond the implicit human
  // "user". Grows as agents author or are addressed. Empty on legacy sessions →
  // treat as [agentId]. (db.Session.Participants; generic participant model.)
  participants?: string[]
  // Broad category of what produced the transcript: chat | task | flow |
  // schedule. Drives the executions feed's kind badge.
  kind: string
  // Links the session to the entity that owns it (a task or flow id); empty for
  // plain chat and agent-keyed kinds (schedule).
  sourceId?: string
  title: string
  messageCount: number
  state: string
  // Session-header format version (db.SessionSchemaVersion); 0 = pre-versioning.
  v?: number
  // Pins the session to the top of the sidebar list regardless of recency.
  pinned?: boolean
  // Free-form labels, editable by the user and agents. Session tags also drive
  // tag-triggered automations (a tagged session finishing a turn fires them).
  tags?: string[]
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
  // Multi-agent coordination (M2, _Docs/47): 'coordinator' | 'worker' | '' (or
  // undefined for an ordinary session). coordinatorSessionId back-links a worker
  // to the coordinator that spawned it. Drives the sidebar's "Workers" filter.
  role?: string
  coordinatorSessionId?: string
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
  // Agent provider ('claude-cli' | 'anthropic' | 'minimax' | …) — drives
  // provider-aware UI notes (tool delivery, dynamic placement).
  provider: string
  multiAgent: boolean
  system: string
  systemTokens: number
  // Selected-skills catalog block, split out of the system prompt (still part of
  // the cached static prefix). Empty when the agent has no skills selected.
  skills: string
  skillsTokens: number
  // Rolling summary (the compacted stand-in for the dropped messages), split out
  // of the dynamic suffix. Empty when the session has no summary. Counts in total.
  summary: string
  summaryTokens: number
  dynamic: string
  dynamicTokens: number
  messages: { role: string; text: string; author?: string; self?: boolean }[]
  messageTokens: number
  // Turns folded into the rolling summary and NO LONGER sent (shown separately).
  // droppedTokens is their size and is NOT part of totalTokens (not on the wire).
  droppedMessages: { role: string; text: string; author?: string; self?: boolean }[]
  droppedTokens: number
  tools: { name: string; description: string; inputSchema?: unknown }[]
  toolTokens: number
  // Lazy (on-demand) tools: schemas NOT shipped at turn start; name+desc only.
  // Their token cost is already inside systemTokens (load-on-demand catalog block).
  // tools + lazyTools = the effective catalog the session/agent info screen counts.
  // visibility is the tool's tier ("summary" | "name-only" | "hidden"); the row is
  // rendered accordingly (summary keeps its description, name-only/hidden name alone).
  lazyTools: { name: string; description: string; visibility?: string }[]
  totalTokens: number
  // Exact prompt size counted by the provider's REAL tokenizer server-side
  // (?accurate=1, anthropic only). 0/absent = not requested or unsupported.
  accurateTokens?: number
  cache: CachePreview
  // Present only for CLI-wrapper providers (claude-cli): the gap
  // between TionSwarm's segment estimate (totalTokens) and the real prompt the CLI
  // sends (its own system + tools + MCP bridge, which TionSwarm never sees).
  cliOverhead?: CLIOverhead
  // True when this preview was requested with compaction simulated (?compact=1):
  // the message array reflects this turn's budgeted fold (read-only, no summary
  // generated/persisted). foldedCount is how many pending messages would fold.
  compactionSimulated: boolean
  foldedCount: number
}

// CLIOverhead surfaces, for CLI-wrapper agents, that totalTokens under-reports the
// real billed input. measuredTokens is ONE call's real model input: claude-cli
// reports in/cache CUMULATIVELY across its internal tool-loop round-trips, so the
// recorded turn total is divided by the round-trip count (calls == result num_turns)
// to recover the per-call figure (0 until the first turn); overheadTokens =
// max(0, measured − estimated). predictedOverhead is the projection from the
// empirically measured reference (base system + built-ins + eager bridged tools),
// available BEFORE the first turn is billed.
export interface CLIOverhead {
  note: string
  estimatedTokens: number
  measuredTokens: number
  overheadTokens: number
  calls: number
  predictedOverhead: number
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
  // Since P2 the rolling summary rides a synthetic head message inside the cached
  // prefix on native providers (a cache READ between folds); on claude-cli it rides
  // the uncached tail. So the summary segment can be marked warm independently.
  summaryCached: boolean
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
  // Free-form labels (also drive tag-triggered automations).
  tags?: string[]
  // Context-reset lineage: the session this one continues (born from /handoff)
  // and the handoff artifact written into this session at reset.
  parentSessionId?: string
  handoffArtifactId?: string
  // Coordinator/worker role (M2): 'coordinator' | 'worker' | ''. A coordinator
  // session gets the coordinator prompt + worker tools; coordinatorSessionId is a
  // worker's back-link to its coordinator.
  role?: string
  coordinatorSessionId?: string
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

  // An in-flight turn (background provider/claude-cli process) for this session,
  // or absent when idle. Drives the "running process" card (stop / restart).
  running?: RunningTurn
  // True when a persistent-pool claude-cli process is kept warm between turns for
  // this session (persistent-pool mode only). Offer to recycle it.
  warmCliProcess: boolean
}

// RunningTurn describes an in-flight turn behind the Session Info panel's process card.
export interface RunningTurn {
  runId: string
  startedAt: number // unix seconds
  autonomous: boolean
  provider?: string
}

// WorkerInfo is one worker's status under a coordinator session (M2).
export interface WorkerInfo {
  sessionId: string
  agentName: string
  title: string
  running: boolean
  summary: string
}
