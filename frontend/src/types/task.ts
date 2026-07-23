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
  lastRunId: string
  lastRunStatus: string
  lastRunAt: number
  createdAt: number
  updatedAt: number
}

export interface Run {
  id: string
  taskId: string
  agentId: string
  // Transcript session this run threaded into, and the assistant message it
  // produced — for deep-linking a board run to its conversation.
  sessionId?: string
  messageId?: string
  status: 'pending' | 'running' | 'success' | 'failure'
  trigger: string
  output: string
  error: string
  createdAt: number
  updatedAt: number
}

export interface Schedule {
  id: string
  agentId: string
  // When set, the schedule runs this flow (with prompt as input) instead of
  // delivering the prompt to agentId.
  flowId?: string
  cronExpr: string
  prompt: string
  nextRunAt: number
  lastRunAt: number
  lastDeliveryStatus: string
  lastDeliveryError: string
  enabled: boolean
  tags?: string[] // free-form organizational labels (editable by user + agents)
  createdAt: number
  // Optional end date (unix seconds); 0/undefined = no end date.
  expiresAt?: number
}

// Automation is a tag-triggered rule: when a session carrying triggerTag finishes
// a turn, its final reply is rendered into promptTemplate and a new session is
// spawned for targetAgentId. When the spawned session carries triggerTag too (the
// default), each completion re-fires the rule — a self-continuing loop bounded by
// maxIterations / cooldownSec / enabled. Surfaced in the Schedules screen.
// Automation trigger kind: 'tag' (a tagged session finishing a turn) or 'board'
// (a kanban card change). '' from older files is treated as 'tag'.
export type AutomationTriggerKind = 'tag' | 'board'
// Board card operation a board automation reacts to.
export type BoardOp = 'any' | 'move' | 'create' | 'update' | 'delete'

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
  targetAgentId: string
  // When set, the automation runs this flow (with the rendered prompt as input)
  // instead of spawning a session for targetAgentId. Per-trigger (no self-loop).
  flowId?: string
  promptTemplate: string // placeholders: {{result}} {{title}} {{tag}} {{sessionId}}
  spawnTags?: string[] // tags applied to the spawned session (default: [triggerTag])
  enabled: boolean
  maxIterations: number // 0 = unlimited
  cooldownSec: number
  expiresAt?: number // optional end date (unix seconds); 0/undefined = no end date
  // Runtime bookkeeping (read-only).
  iterationCount: number
  lastFiredAt?: number
  lastSessionId?: string
  lastError?: string
  createdBy?: string
  createdAt: number
  updatedAt: number
}
