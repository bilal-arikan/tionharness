// Tasks (kanban), their run history and cron schedules.

// Kanban board column state: stored as a string on tasks and matched against
// the workspace's BoardColumnDef list. The five built-in values are kept for
// type-hinting; custom column keys are also valid (lowercase + underscores).
export type BoardState = 'todo' | 'in_progress' | 'review' | 'done' | 'failed' | (string & {})

export interface Task {
  id: string
  title: string
  description: string
  prompt: string
  ownerAgentId: string
  flowId: string
  boardState: BoardState
  dependencies: string
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
  createdAt: number
  // One-shot fields (user-created single-run schedules).
  oneShot?: boolean
  fireAt?: number
}
