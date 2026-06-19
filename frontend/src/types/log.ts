// Captured log records and autonomous runtime events streamed over /api/events.

// A captured log record (application + all workspaces).
export interface LogEntry {
  seq: number
  time: number // unix milliseconds
  level: string // DEBUG | INFO | WARN | ERROR
  message: string
  attrs?: Record<string, string>
}

// AppEvent is an autonomous runtime notification streamed over /api/events.
// `target` carries navigation hints used to deep-link on notification click
// (keys: view, sessionId, taskId, agentId).
export interface AppEvent {
  type: string // task | schedule | agent
  level: 'info' | 'success' | 'error'
  workspaceId: string
  workspaceName?: string
  title: string
  body: string
  target?: Record<string, string>
  time: number // unix seconds
}
