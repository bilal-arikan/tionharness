// Workspace event stream client — the per-workspace twin of sessionStream
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
import { subscribeHubStream } from './hubStream'
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
} as const

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

// subscribeWorkspaceStream opens the active workspace's stream and keeps it
// alive across reconnects. Returns an unsubscribe function.
export function subscribeWorkspaceStream(handlers: WorkspaceStreamHandlers): () => void {
  return subscribeHubStream('/api/workspace/stream', {
    ...handlers,
    onEvent: (ev) => handlers.onEvent(ev as WorkspaceHubEvent),
  })
}

// dataOf narrows a workspace event's data for one kind; undefined when the
// event is of another kind or carries no data.
export function dataOf<D>(ev: WorkspaceHubEvent, kind: WorkspaceStreamKindValue): D | undefined {
  if (ev.kind !== kind) return undefined
  return ev.payload?.data as D | undefined
}
