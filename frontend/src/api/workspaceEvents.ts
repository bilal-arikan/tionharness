// Workspace event stream vocabulary — kinds, payload shapes and the dataOf
// narrowing helper, kept free of the transport (and of api/client, which reads
// localStorage at import) so pure reducers and node-environment tests can use
// them. The live subscription lives in workspaceStream.ts.
//
// The stream itself is the per-workspace twin of sessionStream
// (GET /api/workspace/stream, _Docs/77 R3). One ordered, replayable log of
// structured lifecycle facts for the active workspace: sessions created /
// changed state / deleted, flow runs started / waiting / finished, schedules
// armed for a future fire, automation fires, trajectory revisions. A live
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
  Trajectory: 'trajectory',
  FlowRun: 'flow_run',
  ScheduleArmed: 'schedule_armed',
  AutomationFire: 'automation_fire',
  Spawn: 'spawn',
  Report: 'report',
  Liveness: 'liveness',
  Coordination: 'coordination',
} as const

// ws:spawn — a coordinator spawned a worker (agent.SpawnEvent).
export interface SpawnData {
  coordinatorId: string
  workerId: string
  rootId: string
  depth: number
  agentRef: string
  agentName?: string
  subCoordinator?: boolean
  workflow?: string
  queued?: boolean
  at: number
}

// ws:report — a worker's task-notification reached its coordinator (agent.ReportEvent).
export interface ReportData {
  coordinatorId: string
  workerId: string
  status: string
  lastWorker: boolean
  toolUses: number
  noteBytes: number
  at: number
}

// ws:coordination — a drain turn started/ended (DrainEvent) or the stall guard
// halted the coordinator (StallEvent); narrow on payload.target.phase.
export interface CoordinationData {
  coordinatorId: string
  phase?: 'turn_start' | 'turn_end'
  turn?: number
  reason?: string
  streak?: number
  at: number
}

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
  coordinator?: boolean
  updatedAt: number
}

export interface TrajectoryData {
  trajectoryId: string
  rootSessionId: string
  op: 'create' | 'update' | 'delete'
  templateRef?: string
  status?: string
  revision: number
  nodeCount: number
  updatedAt?: number
}

export interface FlowRunData {
  runId: string
  flowId: string
  parentRunId?: string
  parentNodeId?: string
  rootRunId: string
  sessionId?: string
  status: 'running' | 'waiting' | 'success' | 'failure'
  error?: string
  createdAt: number
  updatedAt: number
}

export interface ScheduleArmedData {
  scheduleId: string
  name?: string
  agentId?: string
  flowId?: string
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
  triggerSessionId?: string
  iterationCount?: number
}

// Every workspace hub event carries this payload: the server's navigation
// hints plus the kind-specific data.
export interface WorkspaceStreamPayload<D = unknown> {
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
