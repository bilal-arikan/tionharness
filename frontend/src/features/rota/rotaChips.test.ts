import { describe, expect, it } from 'vitest'
import type { Session } from '@/types/session'
import { emptyLanes } from '@/shared/lib/laneModel'
import { seedLiveness, seedSessions } from '@/shared/lib/laneReducer'
import { ALL_SESSION_CHIPS } from '@/features/sessions/sessionKindMeta'
import { laneChipCounts, laneChipFilter, laneMatchesChips } from './rotaChips'
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

// ROOT (chat) with a completed worker W1 and a subagent worker W2; SOLO is an
// unrelated archived chat; AUTO is an automation root.
function fixture() {
  return seedSessions(emptyLanes(), [
    header({ id: 'ROOT', coordinatorMode: true }),
    header({
      id: 'W1',
      kind: 'worker',
      runState: 'completed',
      origin: {
        kind: 'coordinator',
        triggerSessionId: 'ROOT',
        rootSessionId: 'ROOT',
        at: NOW - 500,
      },
    }),
    header({
      id: 'W2',
      kind: 'worker',
      origin: { kind: 'subagent', triggerSessionId: 'ROOT', rootSessionId: 'ROOT', at: NOW - 400 },
    }),
    header({ id: 'SOLO', state: 'archived' }),
    header({ id: 'AUTO', kind: 'automation' }),
  ])
}

const all = new Set(ALL_SESSION_CHIPS)
const only = (...keys: string[]) => new Set(keys)

describe('laneMatchesChips', () => {
  it('keeps everything when every chip is on', () => {
    const l = fixture()
    for (const id of ['ROOT', 'W1', 'W2', 'SOLO', 'AUTO']) {
      expect(laneMatchesChips(l, l.sessions.get(id)!, all)).toBe(true)
    }
  })

  it('a worker needs its Worker chip as well as its kind chip', () => {
    const l = fixture()
    expect(laneMatchesChips(l, l.sessions.get('W1')!, only('worker', 'archived'))).toBe(true)
    expect(laneMatchesChips(l, l.sessions.get('W1')!, only('chat', 'archived'))).toBe(false)
  })

  it('classifies a subagent worker by its origin kind', () => {
    const l = fixture()
    expect(laneMatchesChips(l, l.sessions.get('W2')!, only('subagent', 'worker', 'archived'))).toBe(
      true,
    )
    expect(laneMatchesChips(l, l.sessions.get('W2')!, only('worker', 'archived'))).toBe(false)
  })

  it('an archived lane needs the Arşiv chip', () => {
    const l = fixture()
    expect(laneMatchesChips(l, l.sessions.get('SOLO')!, only('chat'))).toBe(false)
    expect(laneMatchesChips(l, l.sessions.get('SOLO')!, only('chat', 'archived'))).toBe(true)
  })

  it('a running lane needs the Çalışan chip', () => {
    const l = seedLiveness(fixture(), {
      entries: [{ sessionId: 'ROOT', state: 'running' }],
      capacity: {},
      at: NOW,
    } as never)
    expect(laneMatchesChips(l, l.sessions.get('ROOT')!, only('chat', 'archived'))).toBe(false)
    expect(laneMatchesChips(l, l.sessions.get('ROOT')!, only('chat', 'archived', 'running'))).toBe(
      true,
    )
  })
})

describe('laneChipFilter', () => {
  it('keeps a root whose member matches, so no worker is orphaned', () => {
    const l = fixture()
    // Only the Subagent slice is asked for. `worker` has to stay on because it
    // is the scope chip every worker lane needs; W1's KIND is also 'worker', so
    // it survives too — the same double duty the sidebar's chip has. What this
    // asserts is the tree rule: ROOT is a chat and matches nothing here, yet it
    // stays so its members' spawn edges still point at a rendered lane.
    const keep = laneChipFilter(l, only('subagent', 'worker', 'archived'))
    expect(keep(l.sessions.get('W2')!)).toBe(true)
    expect(keep(l.sessions.get('ROOT')!)).toBe(true)
    // An unrelated root with no matching member drops out.
    expect(keep(l.sessions.get('AUTO')!)).toBe(false)
    expect(keep(l.sessions.get('SOLO')!)).toBe(false)
  })

  it('drops a root when neither it nor any member matches', () => {
    const l = fixture()
    const keep = laneChipFilter(l, only('automation', 'archived'))
    expect(keep(l.sessions.get('AUTO')!)).toBe(true)
    expect(keep(l.sessions.get('ROOT')!)).toBe(false)
  })

  it('narrows the layout rows it is handed to', () => {
    const l = fixture()
    const rows = (chips: Set<string>) =>
      layoutRota(l, { now: NOW, laneFilter: laneChipFilter(l, chips) }).rows.map((r) => r.id)
    expect(rows(all)).toEqual(['ROOT', 'W1', 'W2', 'SOLO', 'AUTO'])
    expect(rows(only('automation', 'archived'))).toEqual(['AUTO'])
    expect(rows(only('subagent', 'worker', 'archived'))).toEqual(['ROOT', 'W1', 'W2'])
    // Drop the Worker scope and every worker lane goes, root kept on its own.
    expect(rows(only('chat', 'subagent'))).toEqual(['ROOT'])
  })
})

describe('laneChipCounts', () => {
  it('counts a lane once per axis it belongs to', () => {
    const c = laneChipCounts(fixture())
    // ROOT + SOLO are chats; AUTO is an automation. W1's kind chip is 'worker'
    // and W2's is 'subagent' (from its origin); on top of that BOTH answer the
    // Worker scope, so 'worker' totals 3 — kind once, scope twice.
    expect(c.get('chat')).toBe(2)
    expect(c.get('subagent')).toBe(1)
    expect(c.get('automation')).toBe(1)
    expect(c.get('worker')).toBe(3)
    // SOLO is the only archived lane.
    expect(c.get('archived')).toBe(1)
  })

  it('an empty store counts nothing', () => {
    expect(laneChipCounts(emptyLanes()).size).toBe(0)
  })
})
