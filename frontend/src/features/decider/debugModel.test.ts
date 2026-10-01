import { expect, it } from 'vitest'
import type { DeciderDebugEvent } from '@/types/decider'
import { groupDebugEvents } from './debugModel'

const event = (traceId: string, stage: string, at = 10, role = 'primary'): DeciderDebugEvent => ({
  at,
  traceId,
  stage,
  role,
  authority: 'stall-judge',
  mode: 'shadow',
  threshold: 0.7,
})

it('correlates outcomes and preserves append order even at identical timestamps', () => {
  const input = [
    event('a', 'outcome', 11, 'challenger'),
    { ...event('a', 'outcome'), outcome: 'ok' },
    event('a', 'completed'),
    event('a', 'attempt'),
    event('a', 'transport'),
    event('a', 'started'),
  ]
  const [trace] = groupDebugEvents(input)
  expect(trace.events.map((e) => e.stage)).toEqual([
    'started',
    'transport',
    'attempt',
    'completed',
    'outcome',
    'outcome',
  ])
  expect(trace.outcome?.outcome).toBe('ok')
  expect(trace.completed?.stage).toBe('completed')
  expect(input[0].role).toBe('challenger')
})

it('keeps independent traces separate and handles a retained partial tail', () => {
  const traces = groupDebugEvents([event('new', 'attempt', 20), event('old', 'completed', 10)])
  expect(traces.map((t) => t.id)).toEqual(['new', 'old'])
  expect(traces[0].completed).toBeUndefined()
  expect(groupDebugEvents([])).toEqual([])
})
