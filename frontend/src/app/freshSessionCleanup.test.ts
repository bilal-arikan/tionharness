import { describe, expect, it } from 'vitest'
import { shouldDiscardFreshSession, type FreshSessionCleanupInput } from './freshSessionCleanup'

const base: FreshSessionCleanupInput = {
  freshSessionId: 'SES1',
  leavingSessionId: 'SES1',
  messageCount: 0,
  draft: { ok: true, value: '' },
}

describe('shouldDiscardFreshSession', () => {
  it.each([
    ['empty draft', {}, true],
    ['whitespace-only draft', { draft: { ok: true, value: ' \n\t ' } }, true],
    ['draft with text', { draft: { ok: true, value: ' unfinished prompt ' } }, false],
    ['unreadable draft storage', { draft: { ok: false, value: '' } }, false],
    ['sent message', { messageCount: 1 }, false],
    ['non-fresh session', { freshSessionId: 'SES2' }, false],
  ] satisfies ReadonlyArray<[string, Partial<FreshSessionCleanupInput>, boolean]>)(
    '%s',
    (_, input, expected) => {
      expect(shouldDiscardFreshSession({ ...base, ...input })).toBe(expected)
    },
  )
})
