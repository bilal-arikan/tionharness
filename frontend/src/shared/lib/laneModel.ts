// Lane model — the incremental, per-workspace picture the Rota screen renders
// (_Docs/77 R10). It is a plain immutable value: seeded from REST (sessions,
// liveness) and then patched by workspace-stream events (api/workspaceStream.ts)
// through the pure reducer in laneReducer.ts. Every accepted change bumps
// `revision`, so consumers can key memoisation on one number.
import type { SessionOrigin } from '@/types/session'
import type { LivenessCapacity } from '@/types/liveness'
import type { AutomationFireData, ScheduleArmedData } from '@/api/workspaceEvents'

// A session as the lanes know it: the header facts the stream carries, plus
// the last liveness signal. `rootSessionId` is '' for a root (its own lane).
export interface LaneSession {
  id: string
  kind: string
  agentId?: string
  title?: string
  state: string
  runState?: string
  rootSessionId: string
  origin?: SessionOrigin
  createdAt: number
  updatedAt: number
  live?: LaneLive
}

// Turn-admission state of one session (ws:liveness / GET /api/workspace/liveness).
export interface LaneLive {
  state: string
  reason?: string
  since?: number
  waiting?: number
}

// An automation fire as recorded in the lanes (bounded ring, newest last).
export interface LaneFire extends AutomationFireData {
  seq: number
  at: number
}

export interface LaneState {
  // Bumps on every accepted change (seed or event). Stale-event rejections do
  // not bump it, so a rejected event never re-renders anything.
  revision: number
  // Highest stream seq applied so far (0 before the first event).
  head: number
  // Stream connection state, for the "canlı / bağlanıyor" indicator.
  connected: boolean
  // True after the stream reset its cursor: the REST seed must be reloaded
  // before live events are trustworthy again. Cleared by the next seed.
  stale: boolean
  sessions: ReadonlyMap<string, LaneSession>
  armed: ReadonlyMap<string, ScheduleArmedData>
  fires: readonly LaneFire[]
  capacity?: LivenessCapacity
}

export const LANE_FIRE_CAP = 200

export function emptyLanes(): LaneState {
  return {
    revision: 0,
    head: 0,
    connected: false,
    stale: false,
    sessions: new Map(),
    armed: new Map(),
    fires: [],
  }
}

// rootLanes returns the root sessions (own lane), newest activity first.
export function rootLanes(state: LaneState): LaneSession[] {
  const roots: LaneSession[] = []
  for (const s of state.sessions.values()) {
    if (s.rootSessionId === '' || s.rootSessionId === s.id) roots.push(s)
  }
  roots.sort((a, b) => b.updatedAt - a.updatedAt)
  return roots
}

// laneMembers returns the sessions under a root (spawns, subagents, handoffs) in
// creation order (ties by last activity) so the lane reads left-to-right in
// time and a member's trigger always precedes it.
export function laneMembers(state: LaneState, rootId: string): LaneSession[] {
  const out: LaneSession[] = []
  for (const s of state.sessions.values()) {
    if (s.id !== rootId && s.rootSessionId === rootId) out.push(s)
  }
  out.sort((a, b) => a.createdAt - b.createdAt || a.updatedAt - b.updatedAt)
  return out
}
