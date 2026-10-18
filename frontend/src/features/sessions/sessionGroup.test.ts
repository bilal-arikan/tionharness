import { describe, expect, it } from 'vitest'
import type { Session } from '@/types'
import { normalizeSessionGroup, sessionGroupOf } from './sessionGroup'

function mk(over: Partial<Session>): Session {
  return { kind: 'chat', ...over } as Session
}

describe('sessionGroupOf', () => {
  it.each<[string, Partial<Session>, 'main' | 'other']>([
    ['plain user chat', { origin: { kind: 'user', at: 1 } }, 'main'],
    ['legacy chat without origin', {}, 'main'],
    [
      'spawn_session child',
      {
        category: 'chat',
        executionType: 'interactive',
        origin: { kind: 'spawn', triggerSessionId: 'SES1', at: 1 },
      },
      'main',
    ],
    ['handoff continuation', { kind: 'spawned', origin: { kind: 'handoff', at: 1 } }, 'main'],
    ['subagent', { kind: 'subagent', category: 'subagent', visibility: 'internal' }, 'other'],
    ['task stamped chat category', { kind: 'task', category: 'chat' }, 'other'],
    ['insight scan', { kind: 'insight', origin: { kind: 'insight', at: 1 } }, 'other'],
    ['automation-origin chat', { origin: { kind: 'automation', at: 1 } }, 'other'],
    ['system agent call', { tags: ['system:titler'], origin: { kind: 'user', at: 1 } }, 'other'],
    ['system category', { category: 'system' }, 'other'],
  ])('%s', (_, over, want) => {
    expect(sessionGroupOf(mk(over))).toBe(want)
  })
})

describe('normalizeSessionGroup', () => {
  it('defaults anything unknown to the main tab', () => {
    expect(normalizeSessionGroup('other')).toBe('other')
    expect(normalizeSessionGroup('main')).toBe('main')
    expect(normalizeSessionGroup('bogus')).toBe('main')
    expect(normalizeSessionGroup(null)).toBe('main')
  })
})
