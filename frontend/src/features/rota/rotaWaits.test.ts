import { describe, expect, it } from 'vitest'
import type { Session } from '@/types/session'
import { emptyLanes } from '@/shared/lib/laneModel'
import { seedSessions } from '@/shared/lib/laneReducer'
import { clipWaits, formatWait, waitSpans } from './rotaWaits'
import { layoutRota } from './rotaLayout'

const NOW = 100_000

function header(partial: Partial<Session> & { id: string }): Session {
  return {
    agentId: 'AG1',
    kind: 'chat',
    title: partial.id,
    messageCount: 0,
    state: 'active',
    createdAt: NOW - 10_000,
    updatedAt: NOW - 100,
    ...partial,
  } as Session
}

function worker(id: string, createdAt: number, updatedAt: number, live = false): Session {
  return header({
    id,
    kind: 'worker',
    runState: live ? 'running' : 'completed',
    origin: { kind: 'coordinator', triggerSessionId: 'ROOT', rootSessionId: 'ROOT', at: createdAt },
    createdAt,
    updatedAt,
  })
}

function lanes(...workers: Session[]) {
  return seedSessions(emptyLanes(), [
    header({ id: 'ROOT', coordinatorMode: true, createdAt: NOW - 10_000, updatedAt: NOW - 100 }),
    ...workers,
  ])
}

describe('waitSpans', () => {
  it('a single worker becomes one wait over its lifetime', () => {
    const l = lanes(worker('W1', NOW - 8_000, NOW - 6_000))
    expect(waitSpans(l, 'ROOT', NOW)).toEqual([{ start: NOW - 8_000, end: NOW - 6_000, peak: 1 }])
  })

  it('overlapping workers merge into one span, peak counts the concurrency', () => {
    const l = lanes(worker('W1', NOW - 8_000, NOW - 5_000), worker('W2', NOW - 6_000, NOW - 3_000))
    expect(waitSpans(l, 'ROOT', NOW)).toEqual([{ start: NOW - 8_000, end: NOW - 3_000, peak: 2 }])
  })

  it('back-to-back workers read as one continuous wait', () => {
    const l = lanes(worker('W1', NOW - 8_000, NOW - 5_000), worker('W2', NOW - 5_000, NOW - 2_000))
    expect(waitSpans(l, 'ROOT', NOW)).toEqual([{ start: NOW - 8_000, end: NOW - 2_000, peak: 1 }])
  })

  it('a gap between workers splits the wait in two', () => {
    const l = lanes(worker('W1', NOW - 9_000, NOW - 7_000), worker('W2', NOW - 4_000, NOW - 2_000))
    expect(waitSpans(l, 'ROOT', NOW)).toEqual([
      { start: NOW - 9_000, end: NOW - 7_000, peak: 1 },
      { start: NOW - 4_000, end: NOW - 2_000, peak: 1 },
    ])
  })

  it('a live worker keeps its wait open to now', () => {
    const l = lanes(worker('W1', NOW - 3_000, NOW - 2_900, true))
    expect(waitSpans(l, 'ROOT', NOW)).toEqual([{ start: NOW - 3_000, end: NOW, peak: 1 }])
  })

  it('drops waits under a minute and roots with no workers', () => {
    expect(waitSpans(lanes(worker('W1', NOW - 40, NOW - 10)), 'ROOT', NOW)).toEqual([])
    expect(waitSpans(lanes(), 'ROOT', NOW)).toEqual([])
  })

  it('honours the lane filter so a wait never points at a hidden lane', () => {
    const l = lanes(worker('W1', NOW - 8_000, NOW - 6_000), worker('W2', NOW - 4_000, NOW - 2_000))
    const keep = (s: { id: string }) => s.id !== 'W2'
    expect(waitSpans(l, 'ROOT', NOW, keep)).toEqual([
      { start: NOW - 8_000, end: NOW - 6_000, peak: 1 },
    ])
  })
})

describe('clipWaits', () => {
  it('trims a wait that outlives its coordinator bar', () => {
    const waits = [{ start: NOW - 8_000, end: NOW - 1_000, peak: 1 }]
    expect(clipWaits(waits, NOW - 6_000, NOW - 3_000)).toEqual([
      { start: NOW - 6_000, end: NOW - 3_000, peak: 1 },
    ])
  })

  it('drops a wait that no longer clears the minimum after clipping', () => {
    const waits = [{ start: NOW - 8_000, end: NOW - 1_000, peak: 1 }]
    expect(clipWaits(waits, NOW - 1_030, NOW)).toEqual([])
  })
})

describe('layoutRota waits', () => {
  it('puts waits on the root bar only, never on a member', () => {
    const l = lanes(worker('W1', NOW - 8_000, NOW - 6_000))
    const layout = layoutRota(l, { now: NOW, idleCutoffSec: 0 })
    const bar = (id: string) => layout.bars.find((b) => b.id === 'bar:' + id)!
    expect(bar('ROOT').waits).toEqual([{ start: NOW - 8_000, end: NOW - 6_000, peak: 1 }])
    expect(bar('W1').waits).toBeUndefined()
  })

  it('leaves waits unset when a coordinator spawned nothing', () => {
    const layout = layoutRota(lanes(), { now: NOW, idleCutoffSec: 0 })
    expect(layout.bars.find((b) => b.id === 'bar:ROOT')!.waits).toBeUndefined()
  })
})

describe('formatWait', () => {
  it('reads worker count and duration', () => {
    expect(formatWait({ start: 0, end: 600, peak: 1 })).toBe('1 worker · 10 dk bekleme')
    expect(formatWait({ start: 0, end: 8_100, peak: 3 })).toBe('3 worker · 2 sa 15 dk bekleme')
  })
})
