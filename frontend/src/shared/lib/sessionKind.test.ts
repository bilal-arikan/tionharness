import { describe, expect, it } from 'vitest'
import { isWritableSessionKind } from './sessionKind'

describe('isWritableSessionKind', () => {
  // Mirrors writableSessionKindList in internal/db/models.go — an empty kind is
  // the legacy manual chat, so it must stay writable.
  it('accepts the writable kinds', () => {
    expect(isWritableSessionKind('')).toBe(true)
    expect(isWritableSessionKind('chat')).toBe(true)
    expect(isWritableSessionKind('spawned')).toBe(true)
    expect(isWritableSessionKind('schedule')).toBe(true)
    expect(isWritableSessionKind('automation-run')).toBe(true)
    expect(isWritableSessionKind('schedule-run')).toBe(true)
  })

  // Machine-written run logs: readable, but a new user turn has nothing to
  // attach to.
  it('rejects read-only transcripts', () => {
    expect(isWritableSessionKind('insight')).toBe(false)
    expect(isWritableSessionKind('task')).toBe(false)
  })

  // Default deny: a kind added on the backend without updating this list must
  // read as read-only rather than silently show a composer the API rejects.
  it('rejects unknown kinds', () => {
    expect(isWritableSessionKind('something-new')).toBe(false)
  })
})
