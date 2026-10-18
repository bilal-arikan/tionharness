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

// ROOT (chat) with a spawned member W1 and a subagent member W2; SOLO is an
// unrelated archived chat; AUTO is an automation root.
function fixture() {
  return seedSessions(emptyLanes(), [
    header({ id: 'ROOT' }),
    header({
      id: 'W1',
      kind: 'spawned',
      runState: 'completed',
      origin: {
        kind: 'spawn',
        triggerSessionId: 'ROOT',
        rootSessionId: 'ROOT',
        at: NOW - 500,
      },
    }),
    header({
      id: 'W2',
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
      expect(laneMatchesChips(l.sessions.get(id)!, all)).toBe(true)
    }
  })

  it('a member lane is filtered by its own kind chip', () => {
    const l = fixture()
    expect(laneMatchesChips(l.sessions.get('W1')!, only('spawned'))).toBe(true)
    expect(laneMatchesChips(l.sessions.get('W1')!, only('chat', 'archived'))).toBe(false)
  })

  it('classifies a subagent lane by its origin kind', () => {
    const l = fixture()
    expect(laneMatchesChips(l.sessions.get('W2')!, only('subagent'))).toBe(true)
    expect(laneMatchesChips(l.sessions.get('W2')!, only('chat', 'archived'))).toBe(false)
  })

  it('an archived lane needs the Arşiv chip', () => {
    const l = fixture()
    expect(laneMatchesChips(l.sessions.get('SOLO')!, only('chat'))).toBe(false)
    expect(laneMatchesChips(l.sessions.get('SOLO')!, only('chat', 'archived'))).toBe(true)
  })

  it('a running lane needs the Çalışan chip', () => {
    const l = seedLiveness(fixture(), {
      entries: [{ sessionId: 'ROOT', state: 'running' }],
      capacity: {},
      at: NOW,
    } as never)
    expect(laneMatchesChips(l.sessions.get('ROOT')!, only('chat', 'archived'))).toBe(false)
    expect(laneMatchesChips(l.sessions.get('ROOT')!, only('chat', 'archived', 'running'))).toBe(
      true,
    )
  })
})

describe('laneChipFilter', () => {
  it('keeps a root whose member matches, so no member is orphaned', () => {
    const l = fixture()
    // Only the Subagent slice is asked for. ROOT is a chat and matches nothing
    // here, yet it stays so its members' spawn edges still point at a rendered
    // lane.
    const keep = laneChipFilter(l, only('subagent', 'archived'))
    expect(keep(l.sessions.get('W2')!)).toBe(true)
    expect(keep(l.sessions.get('ROOT')!)).toBe(true)
    expect(keep(l.sessions.get('W1')!)).toBe(false)
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
    expect(rows(only('subagent', 'archived'))).toEqual(['ROOT', 'W2'])
    // Only the Sohbet chip: ROOT and the chat-kind W2 stay; SOLO is archived.
    expect(rows(only('chat'))).toEqual(['ROOT'])
  })
})

describe('laneChipCounts', () => {
  it('counts a lane once per axis it belongs to', () => {
    const c = laneChipCounts(fixture())
    // ROOT + SOLO are chats; AUTO is an automation; W1 is spawned and W2 is a
    // subagent (from its origin).
    expect(c.get('chat')).toBe(2)
    expect(c.get('spawned')).toBe(1)
    expect(c.get('subagent')).toBe(1)
    expect(c.get('automation')).toBe(1)
    // SOLO is the only archived lane.
    expect(c.get('archived')).toBe(1)
  })

  it('an empty store counts nothing', () => {
    expect(laneChipCounts(emptyLanes()).size).toBe(0)
  })
})
