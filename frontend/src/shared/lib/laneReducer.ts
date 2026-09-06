// Pure lane reducer (_Docs/77 R10): seeds a LaneState from REST payloads and
// applies workspace-stream events to it. Immutable — every accepted change
// returns a new state with `revision + 1`; a rejected (stale / unknown) event
// returns the SAME object so useSyncExternalStore consumers skip the render.
//
// Stale rejection: the stream is ordered per workspace, but a REST seed can
// land after events that are newer than it, and a reconnect can replay. So a
// session/flow-run event older than what we hold (by updatedAt) is dropped,
// and a trajectory event with a revision not above ours is dropped. Deletes
// always apply.
import type { Session } from '@/types/session'
import type { LivenessSnapshot } from '@/types/liveness'
import type { TrajectoryIndexEntry } from '@/types/trajectory'
import {
  WorkspaceStreamKind,
  dataOf,
  type AutomationFireData,
  type CoordinationData,
  type FlowRunData,
  type LivenessData,
  type ReportData,
  type ScheduleArmedData,
  type SessionLifecycleData,
  type SpawnData,
  type TrajectoryData,
  type WorkspaceHubEvent,
} from '@/api/workspaceEvents'
import {
  LANE_ACTIVITY_CAP,
  LANE_FIRE_CAP,
  type LaneActivity,
  type LaneFire,
  type LaneLive,
  type LaneSession,
  type LaneState,
} from './laneModel'

function bump(state: LaneState, patch: Partial<LaneState>): LaneState {
  return { ...state, ...patch, revision: state.revision + 1 }
}

function withEntry<V>(m: ReadonlyMap<string, V>, k: string, v: V): Map<string, V> {
  const next = new Map(m)
  next.set(k, v)
  return next
}

function withoutEntry<V>(m: ReadonlyMap<string, V>, k: string): Map<string, V> {
  const next = new Map(m)
  next.delete(k)
  return next
}

function pushRing<T>(ring: readonly T[], item: T, cap: number): T[] {
  const next = ring.length >= cap ? ring.slice(ring.length - cap + 1) : ring.slice()
  next.push(item)
  return next
}

// laneSessionFromHeader maps a REST session header to a lane session.
function laneSessionFromHeader(s: Session, live?: LaneLive): LaneSession {
  return {
    id: s.id,
    kind: s.kind,
    agentId: s.agentId,
    title: s.title,
    state: s.state,
    runState: s.runState,
    rootSessionId: s.origin?.rootSessionId ?? s.rootCoordinatorSessionId ?? '',
    origin: s.origin,
    coordinator: s.coordinatorMode,
    createdAt: s.createdAt,
    updatedAt: s.updatedAt,
    live,
  }
}

// seedTrajectories folds REST index rows in, keeping whichever revision is
// higher per trajectory (a stream event may already be ahead of the listing).
export function seedTrajectories(
  state: LaneState,
  rows: readonly TrajectoryIndexEntry[],
): LaneState {
  const next = new Map(state.trajectories)
  let changed = false
  for (const r of rows) {
    const prev = next.get(r.id)
    if (prev && prev.revision >= r.revision) continue
    next.set(r.id, {
      trajectoryId: r.id,
      rootSessionId: r.rootSessionId,
      op: 'update',
      templateRef: r.templateRef,
      status: r.status,
      revision: r.revision,
      nodeCount: r.nodeCount,
      updatedAt: r.updatedAt,
    })
    changed = true
  }
  if (!changed) return state
  return bump(state, { trajectories: next })
}

// seedSessions replaces the session table from a REST listing, keeping the
// liveness of sessions we already hold and dropping sessions the listing no
// longer returns. Clears `stale`.
export function seedSessions(state: LaneState, sessions: readonly Session[]): LaneState {
  const next = new Map<string, LaneSession>()
  for (const s of sessions) {
    const prev = state.sessions.get(s.id)
    // A newer header from the stream wins over an older REST row.
    if (prev && prev.updatedAt > s.updatedAt) {
      next.set(s.id, prev)
      continue
    }
    next.set(s.id, laneSessionFromHeader(s, prev?.live))
  }
  return bump(state, { sessions: next, stale: false })
}

// liveFromEvent converts a ws:liveness signal to a lane live entry, or
// undefined when the session went idle.
function liveFromEvent(d: LivenessData): LaneLive | undefined {
  if (d.busy)
    return { state: d.kind || 'running', reason: d.label, since: d.since, waiting: d.waiting }
  if (d.waiting > 0) return { state: 'queued', waiting: d.waiting }
  return undefined
}

// seedLiveness replaces every session's live entry from the REST snapshot and
// records the spawn capacity.
export function seedLiveness(state: LaneState, snap: LivenessSnapshot): LaneState {
  const byId = new Map<string, LaneLive>()
  for (const e of snap.entries) {
    byId.set(e.sessionId, { state: e.state, reason: e.reason, since: e.since, waiting: e.waiting })
  }
  const next = new Map<string, LaneSession>()
  for (const [id, s] of state.sessions) {
    const live = byId.get(id)
    next.set(id, live === s.live ? s : { ...s, live })
  }
  return bump(state, { sessions: next, capacity: snap.capacity })
}

function applyLifecycle(state: LaneState, d: SessionLifecycleData): LaneState {
  const prev = state.sessions.get(d.sessionId)
  if (d.op === 'delete') {
    if (!prev) return state
    return bump(state, { sessions: withoutEntry(state.sessions, d.sessionId) })
  }
  if (prev && prev.updatedAt > d.updatedAt) return state
  const next: LaneSession = {
    id: d.sessionId,
    kind: d.kind,
    agentId: d.agentId ?? prev?.agentId,
    title: d.title ?? prev?.title,
    state: d.state,
    runState: d.runState ?? prev?.runState,
    rootSessionId: d.rootSessionId ?? prev?.rootSessionId ?? '',
    origin: d.origin ?? prev?.origin,
    coordinator: d.coordinator ?? prev?.coordinator,
    createdAt: d.createdAt || prev?.createdAt || d.origin?.at || d.updatedAt,
    updatedAt: d.updatedAt,
    live: prev?.live,
  }
  return bump(state, { sessions: withEntry(state.sessions, d.sessionId, next) })
}

function applyTrajectory(state: LaneState, d: TrajectoryData): LaneState {
  const prev = state.trajectories.get(d.trajectoryId)
  if (d.op === 'delete') {
    if (!prev) return state
    return bump(state, { trajectories: withoutEntry(state.trajectories, d.trajectoryId) })
  }
  if (prev && prev.revision >= d.revision) return state
  return bump(state, { trajectories: withEntry(state.trajectories, d.trajectoryId, d) })
}

function applyFlowRun(state: LaneState, d: FlowRunData): LaneState {
  const prev = state.flowRuns.get(d.runId)
  if (prev && prev.updatedAt > d.updatedAt) return state
  return bump(state, { flowRuns: withEntry(state.flowRuns, d.runId, d) })
}

function applyArmed(state: LaneState, d: ScheduleArmedData): LaneState {
  const prev = state.armed.get(d.scheduleId)
  if (prev && prev.fireAt === d.fireAt) return state
  return bump(state, { armed: withEntry(state.armed, d.scheduleId, d) })
}

function applyLiveness(state: LaneState, d: LivenessData): LaneState {
  const prev = state.sessions.get(d.sessionId)
  if (!prev) return state
  const live = liveFromEvent(d)
  if (!live && !prev.live) return state
  return bump(state, { sessions: withEntry(state.sessions, d.sessionId, { ...prev, live }) })
}

function applyFire(state: LaneState, ev: WorkspaceHubEvent, d: AutomationFireData): LaneState {
  const fire: LaneFire = { ...d, seq: ev.seq, at: ev.time }
  return bump(state, { fires: pushRing(state.fires, fire, LANE_FIRE_CAP) })
}

function applyActivity(state: LaneState, a: LaneActivity): LaneState {
  return bump(state, { activity: pushRing(state.activity, a, LANE_ACTIVITY_CAP) })
}

// applyLaneEvent folds one workspace-stream event into the state. Unknown
// kinds and stale events return the same state object.
export function applyLaneEvent(state: LaneState, ev: WorkspaceHubEvent): LaneState {
  let next = state
  switch (ev.kind) {
    case WorkspaceStreamKind.SessionLifecycle: {
      const d = dataOf<SessionLifecycleData>(ev, ev.kind)
      if (d) next = applyLifecycle(state, d)
      break
    }
    case WorkspaceStreamKind.Trajectory: {
      const d = dataOf<TrajectoryData>(ev, ev.kind)
      if (d) next = applyTrajectory(state, d)
      break
    }
    case WorkspaceStreamKind.FlowRun: {
      const d = dataOf<FlowRunData>(ev, ev.kind)
      if (d) next = applyFlowRun(state, d)
      break
    }
    case WorkspaceStreamKind.ScheduleArmed: {
      const d = dataOf<ScheduleArmedData>(ev, ev.kind)
      if (d) next = applyArmed(state, d)
      break
    }
    case WorkspaceStreamKind.AutomationFire: {
      const d = dataOf<AutomationFireData>(ev, ev.kind)
      if (d) next = applyFire(state, ev, d)
      break
    }
    case WorkspaceStreamKind.Liveness: {
      const d = dataOf<LivenessData>(ev, ev.kind)
      if (d) next = applyLiveness(state, d)
      break
    }
    case WorkspaceStreamKind.Spawn: {
      const d = dataOf<SpawnData>(ev, ev.kind)
      if (d) {
        next = applyActivity(state, {
          seq: ev.seq,
          kind: ev.kind,
          at: d.at || ev.time,
          coordinatorId: d.coordinatorId,
          workerId: d.workerId,
          status: d.queued ? 'queued' : undefined,
        })
      }
      break
    }
    case WorkspaceStreamKind.Report: {
      const d = dataOf<ReportData>(ev, ev.kind)
      if (d) {
        next = applyActivity(state, {
          seq: ev.seq,
          kind: ev.kind,
          at: d.at || ev.time,
          coordinatorId: d.coordinatorId,
          workerId: d.workerId,
          status: d.status,
        })
      }
      break
    }
    case WorkspaceStreamKind.Coordination: {
      const d = dataOf<CoordinationData>(ev, ev.kind)
      if (d) {
        next = applyActivity(state, {
          seq: ev.seq,
          kind: ev.kind,
          at: d.at || ev.time,
          coordinatorId: d.coordinatorId,
          phase: d.phase ?? ev.payload?.target?.phase,
          reason: d.reason,
        })
      }
      break
    }
    default:
      return state
  }
  if (next === state) return state
  return ev.seq > next.head ? { ...next, head: ev.seq } : next
}

// markConnection records the stream lifecycle without touching the data.
export function markConnection(state: LaneState, connected: boolean): LaneState {
  if (state.connected === connected) return state
  return bump(state, { connected })
}

// markStale flags that the stream cursor was reset: the next seed must reload.
export function markStale(state: LaneState): LaneState {
  if (state.stale) return state
  return bump(state, { stale: true })
}
