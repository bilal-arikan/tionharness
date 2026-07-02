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
export interface Automation {
  id: string
  name: string
  triggerTag: string
  targetAgentId: string
  promptTemplate: string // placeholders: {{result}} {{title}} {{tag}} {{sessionId}}
  spawnTags?: string[] // tags applied to the spawned session (default: [triggerTag])
  enabled: boolean
  maxIterations: number // 0 = unlimited
  cooldownSec: number
  // Runtime bookkeeping (read-only).
  iterationCount: number
  lastFiredAt?: number
  lastSessionId?: string
  lastError?: string
  createdBy?: string
  createdAt: number
  updatedAt: number
}
