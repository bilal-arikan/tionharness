// Workspace event stream vocabulary — kinds, payload shapes and the dataOf
// narrowing helper, kept free of the transport (and of api/client, which reads
// localStorage at import) so pure reducers and node-environment tests can use
// them. The live subscription lives in workspaceStream.ts.
//
// The stream itself is the per-workspace twin of sessionStream
// (GET /api/workspace/stream, _Docs/77 R3). One ordered, replayable log of
// structured lifecycle facts for the active workspace: sessions created /
// changed state / deleted, schedules
// armed for a future fire, automation fires, liveness changes. A live
// workspace view keeps an incremental picture from it and gap-fills after a
// reconnect instead of re-fetching everything.
//
// Contract differences from the session stream: a fresh subscribe replays
// nothing (load the current picture over REST, then apply live events from
// `head`), and `sessionId` on the hub event is empty — the subject is in the
// payload's `target` / `data`.
import type { HubEvent, HubStreamHandlers } from './hubStream'
import type { SessionOrigin } from '@/types/session'

// Kinds mirror internal/sessionhub KindWS* constants.
export const WorkspaceStreamKind = {
  SessionLifecycle: 'session_lifecycle',
  ScheduleArmed: 'schedule_armed',
  AutomationFire: 'automation_fire',
  Liveness: 'liveness',
  MCPStatus: 'mcp_status',
} as const

// One session's turn-admission state right after it changed (ws:liveness).
// The full picture is GET /api/workspace/liveness (LivenessSnapshot).
export interface LivenessData {
  sessionId: string
  busy: boolean
  kind?: string
  label?: string
  since?: number
  waiting: number
}

export type WorkspaceStreamKindValue =
  (typeof WorkspaceStreamKind)[keyof typeof WorkspaceStreamKind]

// Payload shapes mirror internal/agent/wsevents.go.
export interface SessionLifecycleData {
  sessionId: string
  op: 'create' | 'state' | 'runstate' | 'origin' | 'delete'
  kind: string
  agentId?: string
  title?: string
  state: string
  prevState?: string
  runState?: string
  prevRunState?: string
  rootSessionId: string
  origin: SessionOrigin
  createdAt?: number
  updatedAt: number
}

export interface ScheduleArmedData {
  scheduleId: string
  name?: string
  agentId?: string
  oneShot?: boolean
  sessionId?: string
  fireAt: number
}

export interface AutomationFireData {
  automationId: string
  name?: string
  triggerKind: string
  outcome: 'fired' | 'skipped'
  reason?: string
  sessionId?: string
  iterationCount?: number
}

// ws:mcp_status — a pooled MCP server connection changed state. Today the only
// op is "disconnected": an UNEXPECTED mid-session death. Clean shutdowns (idle
// reap, session close, config re-dial, pool close) deliberately emit nothing, so
// receiving this always means a real failure.
//
// `scoped` distinguishes a per-(session,agent) connection from the shared
// workspace-wide one: a scoped death affects only `sessionId`, and must NOT be
// rendered as "the server is down" while other sessions still hold live
// connections. Mirrors agent.MCPStatusPayload.
export interface MCPStatusData {
  op: 'disconnected'
  server: string
  scoped: boolean
  scopeKey?: string
  sessionId?: string
  error?: string
  pendingCalls: number
  at: number
}

// Every workspace hub event carries this payload: the server's navigation
// hints plus the kind-specific data.
interface WorkspaceStreamPayload<D = unknown> {
  target?: Record<string, string>
  data?: D
  level?: string
}

export type WorkspaceHubEvent = HubEvent & { payload?: WorkspaceStreamPayload }

export type WorkspaceStreamHandlers = Omit<HubStreamHandlers, 'onEvent'> & {
  onEvent: (ev: WorkspaceHubEvent) => void
}

// dataOf narrows a workspace event's data for one kind; undefined when the
// event is of another kind or carries no data.
export function dataOf<D>(ev: WorkspaceHubEvent, kind: WorkspaceStreamKindValue): D | undefined {
  if (ev.kind !== kind) return undefined
  return ev.payload?.data as D | undefined
}
