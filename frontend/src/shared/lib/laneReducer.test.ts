import { describe, expect, it } from 'vitest'
import type { Session } from '@/types/session'
import type { WorkspaceHubEvent } from '@/api/workspaceEvents'
import { emptyLanes, laneMembers, rootLanes, trajectoryForRoot } from './laneModel'
import {
  applyLaneEvent,
  markConnection,
  markStale,
  seedLiveness,
  seedSessions,
} from './laneReducer'

function header(partial: Partial<Session> & { id: string }): Session {
  return {
    agentId: 'AG1',
    kind: 'chat',
    title: partial.id,
    messageCount: 0,
    state: 'active',
    createdAt: 1,
    updatedAt: 10,
    ...partial,
  } as Session
}

function ev(
  seq: number,
  kind: string,
  data: unknown,
  target?: Record<string, string>,
): WorkspaceHubEvent {
  return { seq, sessionId: '', kind, time: 1000 + seq, payload: { data, target } }
}

describe('lane reducer', () => {
  it('seeds sessions and groups them into root lanes', () => {
    const s = seedSessions(emptyLanes(), [
      header({ id: 'ROOT', coordinatorMode: true, updatedAt: 5 }),
      header({
        id: 'W1',
        kind: 'worker',
        origin: { kind: 'coordinator', triggerSessionId: 'ROOT', rootSessionId: 'ROOT', at: 1 },
        updatedAt: 6,
      }),
      header({
        id: 'W2',
        kind: 'worker',
        origin: { kind: 'coordinator', triggerSessionId: 'ROOT', rootSessionId: 'ROOT', at: 1 },
        updatedAt: 3,
      }),
      header({ id: 'SOLO', updatedAt: 9 }),
    ])
    expect(s.revision).toBe(1)
    expect(rootLanes(s).map((r) => r.id)).toEqual(['SOLO', 'ROOT'])
    expect(laneMembers(s, 'ROOT').map((m) => m.id)).toEqual(['W2', 'W1'])
    expect(s.stale).toBe(false)
  })

  it('applies lifecycle events and rejects stale ones', () => {
    let s = seedSessions(emptyLanes(), [header({ id: 'A', updatedAt: 10 })])
    const newer = applyLaneEvent(
      s,
      ev(7, 'session_lifecycle', {
        sessionId: 'A',
        op: 'state',
        kind: 'chat',
        state: 'archived',
        rootSessionId: '',
        origin: { kind: 'user', at: 1 },
        updatedAt: 11,
      }),
    )
    expect(newer).not.toBe(s)
    expect(newer.sessions.get('A')?.state).toBe('archived')
    expect(newer.head).toBe(7)
    s = newer
    const stale = applyLaneEvent(
      s,
      ev(8, 'session_lifecycle', {
        sessionId: 'A',
        op: 'state',
        kind: 'chat',
        state: 'active',
        rootSessionId: '',
        origin: { kind: 'user', at: 1 },
        updatedAt: 5,
      }),
    )
    expect(stale).toBe(s)
    expect(stale.head).toBe(7)
    // A later REST seed carrying an older row does not roll the header back.
    const reseeded = seedSessions(s, [header({ id: 'A', updatedAt: 4 })])
    expect(reseeded.sessions.get('A')?.state).toBe('archived')
    // Delete always applies.
    const gone = applyLaneEvent(
      s,
      ev(9, 'session_lifecycle', {
        sessionId: 'A',
        op: 'delete',
        kind: 'chat',
        state: 'active',
        rootSessionId: '',
        origin: { kind: 'user', at: 1 },
        updatedAt: 0,
      }),
    )
    expect(gone.sessions.has('A')).toBe(false)
  })

  it('keeps the highest trajectory revision', () => {
    const base = emptyLanes()
    const r2 = applyLaneEvent(
      base,
      ev(1, 'trajectory', {
        trajectoryId: 'RTA1',
        rootSessionId: 'ROOT',
        op: 'update',
        revision: 2,
        nodeCount: 3,
      }),
    )
    const r1 = applyLaneEvent(
      r2,
      ev(2, 'trajectory', {
        trajectoryId: 'RTA1',
        rootSessionId: 'ROOT',
        op: 'update',
        revision: 1,
        nodeCount: 1,
      }),
    )
    expect(r1).toBe(r2)
    expect(trajectoryForRoot(r2, 'ROOT')?.nodeCount).toBe(3)
    const del = applyLaneEvent(
      r2,
      ev(3, 'trajectory', {
        trajectoryId: 'RTA1',
        rootSessionId: 'ROOT',
        op: 'delete',
        revision: 0,
        nodeCount: 0,
      }),
    )
    expect(del.trajectories.size).toBe(0)
  })

  it('tracks liveness from the snapshot and from events', () => {
    let s = seedSessions(emptyLanes(), [header({ id: 'A' }), header({ id: 'B' })])
    s = seedLiveness(s, {
      entries: [{ sessionId: 'A', state: 'running', reason: 'turn:user' }],
      capacity: {
        spawnActive: 1,
        spawnMax: 4,
        queueDepth: 0,
        queueMax: 8,
        busyTurns: 1,
        autonomyPaused: false,
      },
      at: 1,
    })
    expect(s.sessions.get('A')?.live?.state).toBe('running')
    expect(s.sessions.get('B')?.live).toBeUndefined()
    expect(s.capacity?.spawnActive).toBe(1)
    const idle = applyLaneEvent(s, ev(4, 'liveness', { sessionId: 'A', busy: false, waiting: 0 }))
    expect(idle.sessions.get('A')?.live).toBeUndefined()
    const queued = applyLaneEvent(
      idle,
      ev(5, 'liveness', { sessionId: 'B', busy: false, waiting: 2 }),
    )
    expect(queued.sessions.get('B')?.live).toEqual({ state: 'queued', waiting: 2 })
    // Unknown session: nothing to attach to, same state.
    expect(
      applyLaneEvent(queued, ev(6, 'liveness', { sessionId: 'ZZ', busy: true, waiting: 0 })),
    ).toBe(queued)
  })

  it('rings fires and activity, ignores unknown kinds', () => {
    let s = emptyLanes()
    s = applyLaneEvent(
      s,
      ev(1, 'automation_fire', { automationId: 'AUT1', triggerKind: 'tag', outcome: 'fired' }),
    )
    s = applyLaneEvent(
      s,
      ev(2, 'spawn', {
        coordinatorId: 'C',
        workerId: 'W',
        rootId: 'C',
        depth: 1,
        agentRef: 'x',
        at: 50,
      }),
    )
    s = applyLaneEvent(
      s,
      ev(
        3,
        'coordination',
        { coordinatorId: 'C', reason: 'no progress', at: 60 },
        { phase: 'stall_halt' },
      ),
    )
    expect(s.fires).toHaveLength(1)
    expect(s.fires[0]).toMatchObject({ automationId: 'AUT1', seq: 1, at: 1001 })
    expect(s.activity.map((a) => a.kind)).toEqual(['spawn', 'coordination'])
    expect(s.activity[1].phase).toBe('stall_halt')
    expect(s.head).toBe(3)
    expect(applyLaneEvent(s, ev(4, 'something_else', {}))).toBe(s)
  })

  it('connection flags bump only on change', () => {
    const s = emptyLanes()
    const on = markConnection(s, true)
    expect(on.connected).toBe(true)
    expect(markConnection(on, true)).toBe(on)
    const stale = markStale(on)
    expect(stale.stale).toBe(true)
    expect(markStale(stale)).toBe(stale)
    expect(seedSessions(stale, []).stale).toBe(false)
  })
})
