import { describe, expect, it } from 'vitest'
import type { Session } from '@/types/session'
import type { WorkspaceHubEvent } from '@/api/workspaceEvents'
import { emptyLanes } from '@/shared/lib/laneModel'
import { applyLaneEvent, seedLiveness, seedSessions } from '@/shared/lib/laneReducer'
import { layoutRota } from './rotaLayout'

const NOW = 10_000

function header(partial: Partial<Session> & { id: string }): Session {
  return {
    agentId: 'AG1',
    kind: 'chat',
    title: partial.id,
    messageCount: 0,
    state: 'active',
    createdAt: NOW - 600,
    updatedAt: NOW - 100,
    ...partial,
  } as Session
}

function ev(
  seq: number,
  kind: string,
  data: unknown,
  target?: Record<string, string>,
): WorkspaceHubEvent {
  return { seq, sessionId: '', kind, time: NOW - 50, payload: { data, target } }
}

function fixture() {
  let s = seedSessions(emptyLanes(), [
    header({ id: 'ROOT', coordinatorMode: true, createdAt: NOW - 900, updatedAt: NOW - 10 }),
    header({
      id: 'W1',
      kind: 'worker',
      runState: 'completed',
      origin: {
        kind: 'coordinator',
        triggerSessionId: 'ROOT',
        rootSessionId: 'ROOT',
        at: NOW - 800,
      },
      createdAt: NOW - 800,
      updatedAt: NOW - 300,
    }),
    header({
      id: 'W2',
      kind: 'worker',
      origin: {
        kind: 'coordinator',
        triggerSessionId: 'ROOT',
        rootSessionId: 'ROOT',
        at: NOW - 700,
      },
      createdAt: NOW - 700,
      updatedAt: NOW - 20,
    }),
    header({
      id: 'FORK',
      origin: { kind: 'handoff', triggerSessionId: 'W2', rootSessionId: 'ROOT', at: NOW - 400 },
      createdAt: NOW - 400,
      updatedAt: NOW - 200,
    }),
    header({ id: 'OLD', createdAt: NOW - 90_000, updatedAt: NOW - 80_000 }),
    header({ id: 'SOLO', createdAt: NOW - 200, updatedAt: NOW - 150 }),
  ])
  s = seedLiveness(s, {
    entries: [{ sessionId: 'W2', state: 'running' }],
    capacity: {
      spawnActive: 1,
      spawnMax: 4,
      queueDepth: 0,
      queueMax: 8,
      busyTurns: 1,
      autonomyPaused: false,
    },
    at: NOW,
  })
  s = applyLaneEvent(
    s,
    ev(1, 'automation_fire', {
      automationId: 'AUT1',
      name: 'pano',
      triggerKind: 'tag',
      outcome: 'fired',
      triggerSessionId: 'W1',
    }),
  )
  s = applyLaneEvent(
    s,
    ev(2, 'automation_fire', {
      automationId: 'AUT2',
      triggerKind: 'tag',
      outcome: 'skipped',
      reason: 'cooldown',
      triggerSessionId: 'ZZ',
    }),
  )
  s = applyLaneEvent(
    s,
    ev(3, 'flow_run', {
      runId: 'RUN1',
      flowId: 'FL1',
      rootRunId: 'RUN1',
      sessionId: 'ROOT',
      status: 'running',
      createdAt: NOW - 60,
      updatedAt: NOW - 30,
    }),
  )
  s = applyLaneEvent(
    s,
    ev(4, 'schedule_armed', { scheduleId: 'SCH1', name: 'nightly', fireAt: NOW + 120 }),
  )
  s = applyLaneEvent(s, ev(5, 'schedule_armed', { scheduleId: 'SCH2', fireAt: NOW + 99_999 }))
  s = applyLaneEvent(
    s,
    ev(
      6,
      'coordination',
      { coordinatorId: 'ROOT', reason: 'no progress', at: NOW - 40 },
      { phase: 'stall_halt' },
    ),
  )
  return s
}

describe('layoutRota', () => {
  it('orders rows root-first, members by creation, drops stale idle lanes', () => {
    const l = layoutRota(fixture(), { now: NOW })
    expect(l.rows.map((r) => r.id)).toEqual(['ROOT', 'W1', 'W2', 'FORK', 'SOLO'])
    expect(l.rows.map((r) => r.depth)).toEqual([0, 1, 1, 1, 0])
    expect(l.rows.map((r) => r.y)).toEqual([0, 1, 2, 3, 4])
  })

  it('draws spawn, report and fork edges at the right times', () => {
    const l = layoutRota(fixture(), { now: NOW })
    const byId = Object.fromEntries(l.edges.map((e) => [e.id, e]))
    expect(byId['edge:ROOT>W1']).toMatchObject({ kind: 'spawned', at: NOW - 800 })
    expect(byId['edge:W1>ROOT']).toMatchObject({ kind: 'reported', at: NOW - 300 })
    expect(byId['edge:ROOT>W2']).toMatchObject({ kind: 'spawned' })
    expect(byId['edge:W2>ROOT']).toBeUndefined() // still running
    expect(byId['edge:W2>FORK']).toMatchObject({ kind: 'forked_from', at: NOW - 400 })
  })

  it('bars: live sessions and runs extend to now, finished ones stop at updatedAt', () => {
    const l = layoutRota(fixture(), { now: NOW })
    const bar = (id: string) => l.bars.find((b) => b.id === id)!
    expect(bar('bar:W2')).toMatchObject({ live: true, end: NOW, state: 'running' })
    expect(bar('bar:W1')).toMatchObject({ live: false, end: NOW - 300, state: 'completed' })
    expect(bar('run:RUN1')).toMatchObject({ rowId: 'ROOT', kind: 'flowrun', live: true, end: NOW })
  })

  it('marks fires on the trigger lane, drops unknown lanes, adds stall marks', () => {
    const l = layoutRota(fixture(), { now: NOW })
    expect(l.marks.map((m) => [m.rowId, m.kind])).toEqual([
      ['W1', 'fired'],
      ['ROOT', 'stall'],
    ])
    expect(l.marks[0].label).toBe('pano')
  })

  it('future strip clips to the horizon and window spans the earliest bar', () => {
    const l = layoutRota(fixture(), { now: NOW, futureHorizonSec: 600 })
    expect(l.future.map((f) => [f.id, f.at])).toEqual([
      ['armed:SCH1', NOW + 120],
      ['armed:SCH2', NOW + 600],
    ])
    expect(l.t0).toBe(NOW - 900)
    expect(l.t1).toBe(NOW + 600)
  })

  it('keeps everything when the idle cutoff is off', () => {
    const l = layoutRota(fixture(), { now: NOW, idleCutoffSec: 0 })
    expect(l.rows.map((r) => r.id)).toContain('OLD')
    expect(l.t0).toBe(NOW - 90_000)
  })

  it('empty store yields an empty layout with a one-minute window', () => {
    const l = layoutRota(emptyLanes(), { now: NOW })
    expect(l.rows).toEqual([])
    expect(l.t0).toBe(NOW - 60)
  })
})
