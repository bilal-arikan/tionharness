// Rota F0 layout — pure, deterministic, no layout library (_Docs/77 brief §8.1).
// Turns a LaneState into rows (one per session, roots first with their
// members indented below), time bars, lineage edges, event marks and the
// "future strip" of armed schedules. Everything is in unix SECONDS; the canvas
// maps seconds to pixels, so this module stays testable without a DOM.
//
// Metaphor: git graph. Time runs left to right, the root session is the top
// lane of its group, every spawn opens a lane underneath, a worker that
// reported back draws a return edge to the root lane. Lanes are assigned once
// (row order is by creation time) so a live update never shuffles rows.
import type { LaneFire, LaneSession, LaneState } from '@/shared/lib/laneModel'
import { laneMembers, rootLanes, trajectoryForRoot } from '@/shared/lib/laneModel'
import type { FlowRunData, ScheduleArmedData, TrajectoryData } from '@/api/workspaceEvents'
import { clipWaits, waitSpans, type RotaWait } from './rotaWaits'

export interface RotaRow {
  id: string // session id
  y: number // row index (0-based)
  depth: 0 | 1
  session: LaneSession
  trajectory?: TrajectoryData
}

export interface RotaBar {
  id: string
  rowId: string
  kind: 'session' | 'flowrun'
  start: number
  end: number
  live: boolean
  state: string
  label: string
  run?: FlowRunData
  // Stretches of a coordinator's bar spent waiting on its spawned workers
  // (rotaWaits.ts). Only ever set on a root's session bar.
  waits?: RotaWait[]
}

export interface RotaEdge {
  id: string
  from: string // row id
  to: string // row id
  at: number
  kind: 'spawned' | 'reported' | 'forked_from'
}

export interface RotaMark {
  id: string
  rowId: string
  at: number
  kind: 'fired' | 'skipped' | 'stall'
  label: string
  fire?: LaneFire
}

interface RotaFutureItem {
  id: string
  at: number
  label: string
  schedule: ScheduleArmedData
}

export interface RotaLayout {
  rows: RotaRow[]
  bars: RotaBar[]
  edges: RotaEdge[]
  marks: RotaMark[]
  future: RotaFutureItem[]
  // Time window: t0 = earliest visible start, now = the clock, t1 = end of the
  // future strip (now + horizon, or the latest armed fire if later).
  t0: number
  now: number
  t1: number
}

export interface RotaLayoutOptions {
  now: number
  // How far the future strip reaches past `now` (seconds). Armed schedules
  // beyond it are clipped to the edge.
  futureHorizonSec?: number
  // Idle sessions whose last activity is older than this are dropped from the
  // canvas (seconds). 0 = keep everything the store holds.
  idleCutoffSec?: number
  // Extra lane predicate on top of the idle window (the chip filter). A root
  // that fails it still renders when one of its members passes, so a kept
  // worker never loses the coordinator its spawn edge points at — building
  // that rule is the caller's job (see rotaChips.laneChipFilter).
  laneFilter?: (s: LaneSession) => boolean
}

const DEFAULT_HORIZON = 30 * 60
const DEFAULT_IDLE_CUTOFF = 6 * 60 * 60

function isLive(s: LaneSession): boolean {
  return !!s.live
}

function barEnd(s: LaneSession, now: number): number {
  return isLive(s) ? now : Math.max(s.updatedAt, s.createdAt)
}

function sessionLabel(s: LaneSession): string {
  return s.title || s.id
}

// layoutRota builds the F0 picture. Rows: roots by last activity (newest
// first) — a busy lane rises — then members by creation time (oldest first)
// so a lane reads left-to-right. Cut-off keeps the canvas focused on recent
// work; live sessions are never cut.
export function layoutRota(state: LaneState, opts: RotaLayoutOptions): RotaLayout {
  const now = opts.now
  const horizon = opts.futureHorizonSec ?? DEFAULT_HORIZON
  const cutoff = opts.idleCutoffSec ?? DEFAULT_IDLE_CUTOFF
  const laneFilter = opts.laneFilter
  const inWindow = (s: LaneSession) =>
    isLive(s) || cutoff <= 0 || now - Math.max(s.updatedAt, s.createdAt) <= cutoff
  const keep = (s: LaneSession) => inWindow(s) && (!laneFilter || laneFilter(s))

  const rows: RotaRow[] = []
  const bars: RotaBar[] = []
  const edges: RotaEdge[] = []
  const marks: RotaMark[] = []
  const rowById = new Map<string, RotaRow>()

  const pushRow = (s: LaneSession, depth: 0 | 1, trajectory?: TrajectoryData) => {
    const row: RotaRow = { id: s.id, y: rows.length, depth, session: s, trajectory }
    rows.push(row)
    rowById.set(s.id, row)
    const start = s.createdAt || s.updatedAt
    const end = barEnd(s, now)
    // Only a root can be waiting on workers; a member lane has none of its own.
    const waits = depth === 0 ? clipWaits(waitSpans(state, s.id, now, keep), start, end) : undefined
    bars.push({
      id: 'bar:' + s.id,
      rowId: s.id,
      kind: 'session',
      start,
      end,
      live: isLive(s),
      state: s.live?.state ?? s.runState ?? s.state,
      label: sessionLabel(s),
      waits: waits && waits.length > 0 ? waits : undefined,
    })
  }

  for (const root of rootLanes(state)) {
    const members = laneMembers(state, root.id).filter(keep)
    if (!keep(root) && members.length === 0) continue
    pushRow(root, 0, trajectoryForRoot(state, root.id))
    for (const m of members) {
      pushRow(m, 1)
      const kind = m.origin?.kind
      const trigger = m.origin?.triggerSessionId || root.id
      const fromRow = rowById.get(trigger) ?? rowById.get(root.id)
      if (!fromRow) continue
      const edgeKind: RotaEdge['kind'] =
        kind === 'coordinator' || kind === 'subagent' ? 'spawned' : 'forked_from'
      edges.push({
        id: `edge:${fromRow.id}>${m.id}`,
        from: fromRow.id,
        to: m.id,
        at: m.createdAt || m.updatedAt,
        kind: edgeKind,
      })
      // A finished worker reported back to its coordinator: return edge.
      if (edgeKind === 'spawned' && !isLive(m) && m.runState === 'completed') {
        edges.push({
          id: `edge:${m.id}>${fromRow.id}`,
          from: m.id,
          to: fromRow.id,
          at: m.updatedAt,
          kind: 'reported',
        })
      }
    }
  }

  // Flow runs ride on the lane of the session that launched them.
  for (const run of state.flowRuns.values()) {
    const rowId = run.sessionId && rowById.has(run.sessionId) ? run.sessionId : ''
    if (!rowId) continue
    const live = run.status === 'running' || run.status === 'waiting'
    bars.push({
      id: 'run:' + run.runId,
      rowId,
      kind: 'flowrun',
      start: run.createdAt,
      end: live ? now : run.updatedAt,
      live,
      state: run.status,
      label: run.flowId,
      run,
    })
  }

  // Automation fires mark the lane they were triggered from (or produced).
  for (const f of state.fires) {
    const rowId = pickRow(rowById, f.triggerSessionId, f.sessionId)
    if (!rowId) continue
    marks.push({
      id: 'fire:' + f.seq,
      rowId,
      at: f.at,
      kind: f.outcome === 'fired' ? 'fired' : 'skipped',
      label: (f.name || f.automationId) + (f.reason ? ` · ${f.reason}` : ''),
      fire: f,
    })
  }
  for (const a of state.activity) {
    if (a.phase !== 'stall_halt') continue
    if (!rowById.has(a.coordinatorId)) continue
    marks.push({
      id: 'stall:' + a.seq,
      rowId: a.coordinatorId,
      at: a.at,
      kind: 'stall',
      label: a.reason ? `stall · ${a.reason}` : 'stall',
    })
  }

  const future: RotaFutureItem[] = []
  for (const s of state.armed.values()) {
    if (s.fireAt < now) continue
    future.push({
      id: 'armed:' + s.scheduleId,
      at: Math.min(s.fireAt, now + horizon),
      label: s.name || s.scheduleId,
      schedule: s,
    })
  }
  future.sort((a, b) => a.at - b.at)

  let t0 = now
  for (const b of bars) if (b.start < t0) t0 = b.start
  for (const m of marks) if (m.at < t0) t0 = m.at
  if (t0 === now) t0 = now - 60
  const t1 = now + horizon

  return { rows, bars, edges, marks, future, t0, now, t1 }
}

function pickRow(rows: Map<string, RotaRow>, ...ids: (string | undefined)[]): string {
  for (const id of ids) if (id && rows.has(id)) return id
  return ''
}
