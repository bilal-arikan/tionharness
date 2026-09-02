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
  // Stable execution classification. New UI classification must prefer these
  // fields over legacy kind; old persisted sessions may omit both.
  executionType?: string
  category?: string
  contextMode?: string
  visibility?: string
  targetProfile?: string
  targetAgentId?: string
  // Links the session to the entity that owns it (a task or flow id); empty for
  // plain chat and agent-keyed kinds (schedule).
  sourceId?: string
  title: string
  messageCount: number
  // Visibility only: 'active' | 'archived'. NOT the run outcome — see runState.
  state: string
  // How the last background work turn ended: 'completed' | 'failed' | 'killed' |
  // 'timeout' | 'incomplete'. Runtime-owned and read-only — the API never accepts
  // it as input. Absent on sessions that never ran a background turn.
  runState?: string
  // When runState was last written (unix seconds).
  runStateAt?: number
  // Session-header format version (db.SessionSchemaVersion); 0 = pre-versioning.
  v?: number
  // The model this session last ran with (per-turn history in Message.Model).
  // Populated at session creation and updated each turn; O(1) answer to
  // "which model is this conversation running on?".
  model?: string
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
  // Multi-agent coordination (M2, _Docs/47). Two INDEPENDENT axes since the
  // unlimited-depth rework: `role` is lineage ('worker' = spawned by a
  // coordinator, or '' ), `coordinatorMode` is the capability (drives workers).
  // A mid-level node of a deep tree has both. coordinatorSessionId back-links to
  // the coordinator above; rootCoordinatorSessionId/coordinatorDepth place the
  // session in its tree. Never test `role === 'coordinator'` — use
  // isCoordinatorSession() from shared/lib/coordination.
  role?: string
  coordinatorMode?: boolean
  coordinatorSessionId?: string
  rootCoordinatorSessionId?: string
  coordinatorDepth?: number
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
  // Whether a git binary exists on the machine at all (path-independent). Lets the
  // UI distinguish "git is not installed" from "this folder is not a repo yet".
  gitInstalled: boolean
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
  coordinatorSessionId?: string
  rootCoordinatorSessionId?: string
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
  // between TionHarness's segment estimate (totalTokens) and the real prompt the CLI
  // sends (its own system + tools + MCP bridge, which TionHarness never sees).
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
  // Backward-compatible aliases for chatMeasuredTokens/chatCalls.
  measuredTokens: number
  overheadTokens: number
  calls: number
  chatMeasuredTokens: number
  chatCalls: number
  workerMeasuredTokens: number
  workerCalls: number
  workerKind: string
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
  model?: string
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
  // Pinned to the top of the sidebar list; toggled from the panel's tool buttons.
  pinned?: boolean
  agentId: string
  agentName: string
  messageCount: number
  unread: boolean
  executionType?: string
  category?: string
  contextMode?: string
  targetProfile?: string
  targetAgentId?: string
  runState?: string
  runStateAt?: number
  terminal?: boolean
  durationMs?: number
  inputTokens?: number
  outputTokens?: number
  toolCallCount?: number
  stopReason?: string
  // Stable machine reason only. Backend deliberately excludes raw error text and
  // tool payloads from this detail surface.
  errorSummary?: string
  persistedSteps?: number
  // Free-form labels (also drive tag-triggered automations).
  tags?: string[]
  // Context-reset lineage: the session this one continues (born from /handoff)
  // and the handoff artifact written into this session at reset.
  parentSessionId?: string
  handoffArtifactId?: string
  // Coordination (M2). `role` is lineage ('worker' | ''), `coordinatorMode` the
  // capability; see the Session type above. coordinatorSessionId back-links to
  // the coordinator above, root/depth place this session in its tree.
  role?: string
  coordinatorMode?: boolean
  coordinatorSessionId?: string
  rootCoordinatorSessionId?: string
  coordinatorDepth?: number
  // Selected coordinator recipe/workflow slug (M5), if any.
  coordinatorWorkflow?: string
  // True when the phantom-spawn stall guard hard-halted this coordinator's auto-turns
  // (see coordination_stall.go). Drives the persistent "durduruldu" badge + resume CTA.
  coordinatorStallHalted?: boolean
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
  // The model this session last ran with (per-turn history in Message.Model).
  // Populated at session creation and updated each turn; O(1) answer to
  // "which model is this conversation running on?".
  model?: string
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
  // Liveness, from the same signal the queue watchdog judges on. Elapsed time
  // alone cannot tell a working turn from a wedged one; silence can. Tick
  // lastActivityAt against the server clock rather than trusting a fetched
  // duration — the panel only refetches when the conversation changes, so a
  // silent session (the case that matters) would never update it.
  lastActivityAt: number // deprecated alias
  lastProgressAt: number // unix seconds
  lastProgressKind: string
  progressSequence: number
  idleLimitSec: number // inactivity window that will cancel the turn
  hardLimitSec?: number // deprecated compatibility field; normal flow omits it
}

// WorkerInfo is one worker's status under a coordinator session (M2).
export interface WorkerInfo {
  sessionId: string
  agentName: string
  // The worker agent's identity, so a worker can be rendered with the shared
  // AgentIdentity component (avatar + name + id + model) rather than a bare name.
  // All empty when the worker's agent row no longer exists — the UI then falls
  // back to the session title/id.
  agentId?: string
  agentAvatar?: string
  agentColor?: string
  agentProvider?: string
  agentModel?: string
  // True for a soft-deleted agent, whose sessions outlive it; AgentIdentity
  // badges it instead of showing a normal-looking agent.
  agentDeleted?: boolean
  title: string
  running: boolean
  summary: string
  // Unix seconds when a RUNNING worker's current turn began, for live elapsed
  // time. 0 when the worker is finished, or when its start time is unknown (a
  // turn opened directly on the worker session, or one that survived a restart)
  // — the UI then omits the duration rather than showing a bogus one.
  startedAt: number
  // True for a SUB-COORDINATOR that is "running" only in the sense that its own
  // workers are: it has no turn of its own in flight, it is waiting on its
  // branch. Shown as "delegating" rather than "running", because there is no live
  // turn whose elapsed time would mean anything.
  delegating?: boolean
  // True while a follow-up (send_to_worker) is parked in this worker's single-slot
  // queue, waiting for the current turn to finish. Shown as a "queued" badge so the
  // coordinator sees the message landed and will be delivered, not lost. Only
  // meaningful while running.
  queued?: boolean
}

// CoordinatorTreeNode is one session in a coordinator tree
// (GET /api/sessions/{id}/coordinator-tree), breadth-first from the root.
export interface CoordinatorTreeNode {
  sessionId: string
  agentName: string
  title: string
  // Distance from the tree root (root = 0).
  depth: number
  // The coordinator this node reports to; empty on the root.
  parentSessionId: string
  // Whether this node drives workers of its own (a root or a mid-level node).
  isCoordinator: boolean
  state: string
  // Whether a turn is in flight on this session right now.
  running?: boolean
  // A parked send_to_worker follow-up waiting for this node's current turn to end
  // (single-slot per worker). Shown as a "queued" marker so a message on a deep
  // node is visible from the root. Only meaningful while running.
  queued?: boolean
  // Branch health, derived from the session's auto-tags: 'stuck' (autonomous
  // turns refused — needs a human), 'error' (last turn failed), '' (fine).
  // Surfaced in the tree because the deeper a failure sits, the less likely
  // anyone opens the session it happened in.
  health?: string
  // Still owes its coordinator an upward report. A node parked here is the one
  // shape of "silently blocking everything above it".
  reportPending?: boolean
  // Phantom-spawn hard-halt: this coordinator's auto-turns are stopped until a human
  // resumes it (distinct from generic 'stuck' — it has a one-click resume action).
  stallHalted?: boolean
  // This node's own lifetime spend. Absent when it has no recorded usage.
  calls?: number
  tokens?: number
  costUSD?: number
}

// CoordinatorTree is the whole tree plus its rolled-up spend. The total matters
// because billing is per-session: without it a deep fan-out's real cost is spread
// across descendants nobody opens.
export interface CoordinatorTree {
  rootSessionId: string
  nodes: CoordinatorTreeNode[]
  totalCostUSD: number
  totalSavingsUSD: number
  priced: boolean
  estimated: boolean
}

// CoordinatorAncestor is one step of the upward breadcrumb from a worker
// (GET /api/sessions/{id}/coordinator-ancestors), root first.
export interface CoordinatorAncestor {
  sessionId: string
  agentName: string
  title: string
  depth: number
}
