import { describe, expect, it } from 'vitest'
import type { ViewGraphResult, ViewRef } from '@/types'
import {
  attentionMatches,
  bearsAttention,
  changedKeys,
  facetsOf,
  FLASH_MS,
  mergeLive,
  pruneFlashes,
  relativeAge,
} from './explorerAttention'

const s1: ViewRef = { kind: 'session', id: 'SES1' }
const s2: ViewRef = { kind: 'session', id: 'SES2' }
const base: ViewGraphResult = {
  nodes: [{ label: 'a', ref: s1 }],
  edges: [],
  live: [{ session: s1, state: 'running', agent: { id: 'AG1', name: 'a' } }],
  attention: { 'session:SES2': { level: 'danger', reasons: ['stuck'] } },
}

describe('attention facets', () => {
  it('groups reason codes into the strip facets', () => {
    expect(facetsOf({ level: 'warn', reasons: ['waiting-ask', 'blocked'] })).toEqual([
      'waiting',
      'stuck',
    ])
    expect(attentionMatches({ level: 'danger', reasons: ['failed-card'] }, ['failed'])).toBe(true)
    expect(attentionMatches({ level: 'danger', reasons: ['failed-card'] }, ['stale'])).toBe(false)
    expect(attentionMatches(undefined, ['stale'])).toBe(false)
    expect(attentionMatches(undefined, [])).toBe(true)
  })

  it('marks only sessions and board cards as attention bearers', () => {
    expect(bearsAttention(s1)).toBe(true)
    expect(bearsAttention({ kind: 'board', id: 'board', sub: 'TSK1' })).toBe(true)
    expect(bearsAttention({ kind: 'board', id: 'board' })).toBe(false)
    expect(bearsAttention({ kind: 'category', id: 'sessions' })).toBe(false)
  })
})

describe('live merge and spotlight', () => {
  it('replaces the volatile layers and keeps the structure', () => {
    const merged = mergeLive(base, {
      live: [],
      meta: {},
      attention: {},
      status: {
        at: 1,
        running: 0,
        waiting: 0,
        stuck: 0,
        failedCards: 0,
        failedRuns: 0,
        stale: 0,
        notesToday: 2,
      },
    })
    expect(merged.nodes).toBe(base.nodes)
    expect(merged.live).toEqual([])
    expect(merged.attention).toEqual({})
    expect(merged.status?.notesToday).toBe(2)
  })

  it('spots nodes whose ring or glow changed', () => {
    const next: ViewGraphResult = {
      ...base,
      live: [{ session: s2, state: 'running', agent: { id: 'AG1', name: 'a' } }],
      attention: {
        'session:SES2': { level: 'danger', reasons: ['stuck', 'waiting-ask'] },
        'board:board#TSK1': { level: 'danger', reasons: ['failed-card'] },
      },
    }
    expect(changedKeys(base, next).sort()).toEqual([
      'board:board#TSK1',
      'session:SES1',
      'session:SES2',
    ])
    expect(changedKeys(null, next)).toEqual([])
    expect(changedKeys(base, base)).toEqual([])
  })

  it('prunes expired flashes and keeps the map identity otherwise', () => {
    const flashes = new Map([
      ['a', 1000],
      ['b', 1000 + FLASH_MS],
    ])
    expect(pruneFlashes(flashes, 1000 + FLASH_MS - 1)).toBe(flashes)
    const pruned = pruneFlashes(flashes, 1000 + FLASH_MS)
    expect([...pruned.keys()]).toEqual(['b'])
  })

  it('buckets the digest age', () => {
    const now = 10_000_000 * 1000
    expect(relativeAge(undefined, now)).toBeNull()
    expect(relativeAge(10_000_000 - 30, now)).toEqual({ unit: 'now', count: 0 })
    expect(relativeAge(10_000_000 - 600, now)).toEqual({ unit: 'minutes', count: 10 })
    expect(relativeAge(10_000_000 - 7200, now)).toEqual({ unit: 'hours', count: 2 })
    expect(relativeAge(10_000_000 - 3 * 86400, now)).toEqual({ unit: 'days', count: 3 })
  })
})
