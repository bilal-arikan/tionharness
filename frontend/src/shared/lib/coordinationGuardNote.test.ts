import { describe, expect, it } from 'vitest'
import { dropGuardNotes, GUARD_NOTE_ORIGIN } from './coordinationGuardNote'

const msgs = [
  { id: 'a', origin: undefined },
  { id: 'b', origin: GUARD_NOTE_ORIGIN },
  { id: 'c', origin: 'worker-note' },
]

describe('dropGuardNotes', () => {
  it('removes guard notes when hidden', () => {
    expect(dropGuardNotes(msgs, false).map((m) => m.id)).toEqual(['a', 'c'])
  })

  it('keeps guard notes when shown', () => {
    expect(dropGuardNotes(msgs, true)).toBe(msgs)
  })

  it('returns the same array when there is nothing to drop', () => {
    const clean = [{ id: 'a', origin: 'worker-note' }]
    expect(dropGuardNotes(clean, false)).toBe(clean)
  })
})
