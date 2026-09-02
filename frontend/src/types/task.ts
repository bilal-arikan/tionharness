// Tasks (kanban), their run history and cron schedules.

// Kanban board column state: stored as a string on tasks and matched against
// the workspace's BoardColumnDef list. The five built-in values are kept for
// type-hinting; custom column keys are also valid (lowercase + underscores).
export type BoardState = 'todo' | 'in_progress' | 'review' | 'done' | 'failed' | (string & {})

// Task priority levels (obsidian-pm compatible). '' = unset.
export type TaskPriority = 'critical' | 'high' | 'medium' | 'low' | ''

export interface Task {
  id: string
  title: string
  description: string
  prompt: string
  ownerAgentId: string
  flowId: string
  boardState: BoardState
  dependencies: string
  // Rich card attributes (optional; absent on older tasks).
  priority?: TaskPriority
  tags?: string[]
  // Referenced workspace artifacts (files dropped on the card become artifacts,
  // or existing artifacts linked from the editor). Order is user-meaningful.
  artifactIds?: string[]
  // Archived cards are hidden from the active board (reversible soft-hide). Only
  // present/true on archived cards; the default board list omits them entirely.
  archived?: boolean
  // How many times this card has returned from the review column to a working
  // one — i.e. how many verification rounds it has FAILED. Server-maintained
  // (db.MoveTask); absent/0 on a card that has never been sent back. Rendered as
  // the review-gate badge (see reviewGate.ts).
  reviewBounces?: number
  lastRunId: string
  lastRunStatus: string
  lastRunAt: number
  createdAt: number
  updatedAt: number
}

// The editable subset of a Task, as accepted by PUT /api/tasks/:id. Named so
// callers that build a patch (the board's drag handlers, the form modal) share
// one type instead of re-deriving the Pick inline.
export type TaskPatch = Partial<
  Pick<
    Task,
    | 'title'
    | 'description'
    | 'ownerAgentId'
    | 'flowId'
    | 'boardState'
    | 'dependencies'
    | 'priority'
    | 'tags'
    | 'artifactIds'
  >
>

// Schedule session mode: 'reuse' keeps appending to the agent's single schedule
// thread, 'spawn' opens a new session on every fire. '' resolves to 'reuse'.
export type ScheduleSessionMode = 'reuse' | 'spawn'

export interface Schedule {
  id: string
  // Optional human-readable name (shown in the board card and modal).
  name?: string
  agentId: string
  // When set, the schedule runs this flow (with prompt as input) instead of
  // delivering the prompt to agentId.
  flowId?: string
  cronExpr: string
  prompt: string
  // How an agent-backed schedule uses sessions: 'reuse' (default, '' is treated
  // the same) appends every fire to the agent's one long-lived schedule thread;
  // 'spawn' opens a fresh session per fire. Ignored for flow-backed schedules.
  sessionMode?: ScheduleSessionMode
  nextRunAt: number
  lastRunAt: number
  lastDeliveryStatus: string
  lastDeliveryError: string
  enabled: boolean
  // Archived: out of the cron table and hidden from the default list, restorable.
  archived?: boolean
  tags?: string[] // free-form organizational labels (editable by user + agents)
  createdAt: number
  // Optional end date (unix seconds); 0/undefined = no end date.
  expiresAt?: number
  // One-shot wake (schedule_wake): fires once at fireAt instead of on a cron.
  // Such a row is only listed once its delivery was attempted and failed — it is
  // read-only in the UI and can only be deleted.
  oneShot?: boolean
  // Wake fire time (unix seconds); only meaningful when oneShot is true.
  fireAt?: number
}

// Automation is a tag-triggered rule: when a session carrying triggerTag finishes
// a turn, its final reply is rendered into promptTemplate and a new session is
// spawned for targetAgentId. When the spawned session carries triggerTag too (the
// default), each completion re-fires the rule — a self-continuing loop bounded by
// maxIterations / cooldownSec / enabled. Surfaced in the Schedules screen.
// Automation trigger kind: 'tag' (a tagged session finishing a turn), 'board'
// (a kanban card change), 'token' (cumulative token spend crossing a threshold),
// or 'counter' (a session's message/tool count crossing an interval). '' from
// older files is treated as 'tag'.
export type AutomationTriggerKind = 'tag' | 'board' | 'token' | 'counter'
// Board card operation a board automation reacts to.
export type BoardOp = 'any' | 'move' | 'create' | 'update' | 'delete'
// Token automation scope: one session's lifetime spend, or the whole workspace's
// spend for the current day. '' is treated as 'session'.
export type TokenScope = 'session' | 'workspace'
// Counter automation metric: count user/assistant messages, or executed tool
// calls, in a session. '' is treated as 'message'.
export type CounterMetric = 'message' | 'tool'
// Automation session strategy (agent-backed only): 'spawn' runs a fresh session
// per fire; 'continue' reuses one persistent per-automation thread (history-aware).
// '' resolves per kind (tag/board → spawn, token/counter → continue).
export type SessionMode = 'spawn' | 'continue'
// Counter automation scope: the crossing session's own counter, or the whole
// workspace's cumulative counter (sum of every session). '' is treated as
// 'session'.
export type CounterScope = 'session' | 'workspace'
// What a board automation does when it fires: 'spawn' (default, '' is treated the
// same) runs the target agent/flow — the board drives execution; 'archive'
// archives the card with no LLM call (the 'done → archive' cleanup).
export type BoardAction = 'spawn' | 'archive' | 'move'

// One line of an automation's fire ledger (GET /api/automations/{id}/fires,
// newest first): every attempt, fired or not, with the guard's reason.
export interface AutomationFireRecord {
  at: number
  outcome: 'fired' | 'skipped' | 'failed'
  // skipped: archived | disabled | expired | cooldown | max_iterations |
  // absolute_backstop | autonomy_paused | target_missing | empty_prompt
  reason?: string
  triggerKind?: string
  triggerSessionId?: string
  sessionId?: string
  driver?: 'session' | 'flow'
  error?: string
  iteration?: number
}

export interface Automation {
  id: string
  name: string
  // Defaults to 'tag' when absent (backward compatible).
  triggerKind?: AutomationTriggerKind
  triggerTag: string
  // Board-trigger fields (only meaningful when triggerKind === 'board').
  boardOp?: BoardOp
  boardFromState?: string
  boardToState?: string
  // Fire order among board automations matching the SAME card change; lower runs
  // first (default 0). Sequences two rules on one column instead of racing them.
  boardPriority?: number
  // When true this automation claims the matching card change alone: every other
  // matching board rule is suppressed ("single owner per column").
  boardExclusive?: boolean
  // What firing does: 'spawn' (default) runs the target — the board drives
  // execution; 'archive' archives the card with no LLM call (needs no target).
  boardAction?: BoardAction
  // Explicit destination for a targetless move action.
  boardMoveToState?: string
  // Token-trigger fields (only meaningful when triggerKind === 'token').
  tokenScope?: TokenScope // default 'session'
  // Token interval: fires each time cumulative spend crosses another multiple
  // (e.g. 100000 → at 100k, 200k…). Min 1000. Tokens = input+output+cache.
  tokenThreshold?: number
  // Counter-trigger fields (only meaningful when triggerKind === 'counter').
  counterMetric?: CounterMetric // default 'message'
  counterScope?: CounterScope // default 'session'
  // Count interval: fires each time the watched counter crosses another multiple
  // (e.g. 10 → at 10, 20…). Min 2.
  counterInterval?: number
  // Session strategy for an agent-backed automation (default resolves per kind).
  sessionMode?: SessionMode
  targetAgentId: string
  // When set, the automation runs this flow (with the rendered prompt as input)
  // instead of spawning a session for targetAgentId. Per-trigger (no self-loop).
  flowId?: string
  promptTemplate: string // placeholders: {{result}} {{title}} {{tag}} {{sessionId}}
  spawnTags?: string[] // tags applied to the spawned session (default: [triggerTag])
  enabled: boolean
  // Archived: hidden from the default list and inert, restorable (the curator's
  // archive-only rule, _Docs/77 R5). Distinct from enabled.
  archived?: boolean
  maxIterations: number // range 1-500; 0/unlimited rejected on write (legacy <=0 rows bounded by backstop)
  cooldownSec: number
  expiresAt?: number // optional end date (unix seconds); 0/undefined = no end date
  // Runtime bookkeeping (read-only).
  iterationCount: number
  lastFiredAt?: number
  lastSessionId?: string
  lastError?: string
  createdBy?: string
  // Stable identity of a built-in default rule provisioned into every workspace
  // (EnsureDefaultBoardAutomations). Empty for user/agent-created rules. Template
  // export skips these — the installing workspace makes its own copy.
  seed?: string
  createdAt: number
  updatedAt: number
}
