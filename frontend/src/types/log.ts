// Captured log records and autonomous runtime events streamed over /api/events.
import type { FlowNodeEvent } from './flow'

// A captured log record (application + all workspaces). component/session/
// agent/workspace are first-class source fields promoted by the backend from
// same-named slog attrs, enabling exact filtering without substring search.
export interface LogEntry {
  seq: number
  time: number // unix milliseconds
  level: string // DEBUG | INFO | WARN | ERROR
  message: string
  component?: string // originating subsystem (api/agent/scheduler/mcp/db/…)
  session?: string // session id, when the record carries one
  agent?: string // agent id, when the record carries one
  workspace?: string // workspace id, when the record carries one
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
  // Present only on type === 'session_step' frames (delivered under the SSE
  // `step` event name): the marshalled TurnStep of an in-progress turn. Raw here
  // to avoid a type cycle; the consumer parses it.
  step?: unknown
  // Present only on type === 'flow_node' frames (delivered under the SSE
  // `flownode` event name): one flow node's live lifecycle for the run in
  // target.flowRunId. Typed (no cycle: flow.ts holds no back-reference here).
  node?: FlowNodeEvent
  // Present only on type === 'log' frames (delivered under the SSE `log`
  // event name): one captured application log record for live tailing.
  log?: LogEntry
}
