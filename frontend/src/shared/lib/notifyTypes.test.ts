import { describe, expect, it } from 'vitest'
import { NOTIFY_TYPES, cueForType, notifyTypeMeta, viewIdForType } from './notifyTypes'

// The backend half of this contract (every kind in events.NotifyKinds appears
// here, and nothing extra does) is enforced from Go in
// internal/events/notifyparity_test.go, which parses this file. These tests
// cover the frontend-side behaviour that derives from the list: sound cues, nav
// badges, and the legacy `task` alias.

describe('NOTIFY_TYPES registry', () => {
  it('gives every entry a label and a hint so no toggle renders blank in Settings', () => {
    for (const t of NOTIFY_TYPES) {
      expect(t.label.trim(), `label for ${t.type}`).not.toBe('')
      expect(t.hint.trim(), `hint for ${t.type}`).not.toBe('')
    }
  })

  it('declares each type exactly once (the byType Map would keep only the last)', () => {
    const types = NOTIFY_TYPES.map((t) => t.type)
    expect(new Set(types).size).toBe(types.length)
  })

  it('uses only the three known cue values', () => {
    for (const t of NOTIFY_TYPES) {
      expect(['done', 'ask', null]).toContain(t.cue)
    }
  })
})

describe('cueForType', () => {
  it('plays the reply-ready chime for a completed assistant turn', () => {
    expect(cueForType('chat')).toBe('done')
  })

  it('plays the attention cue when the turn is blocked on the user', () => {
    expect(cueForType('prompt')).toBe('ask')
  })

  it('stays silent for background outcomes', () => {
    expect(cueForType('board')).toBeNull()
    expect(cueForType('artifact')).toBeNull()
    expect(cueForType('anomaly')).toBeNull()
  })

  it('stays silent for an unregistered type instead of throwing', () => {
    expect(cueForType('session_step')).toBeNull()
    expect(cueForType('')).toBeNull()
  })
})

describe('viewIdForType', () => {
  it('maps a type to the nav view whose unread dot it lights', () => {
    expect(viewIdForType('chat')).toBe('chat')
    expect(viewIdForType('flow')).toBe('flows')
    expect(viewIdForType('schedule')).toBe('schedules')
    expect(viewIdForType('artifact')).toBe('artifacts')
  })

  // Task changes are emitted as `board` events; there is no distinct `task`
  // event type. Dropping this alias would silently stop badging the board.
  it('routes the legacy `task` alias to the board view', () => {
    expect(viewIdForType('task')).toBe('board')
    expect(viewIdForType('board')).toBe('board')
  })

  it('returns null for app-global types that carry no badge', () => {
    expect(viewIdForType('coordination')).toBeNull()
    expect(viewIdForType('automation')).toBeNull()
    expect(viewIdForType('agent')).toBeNull()
    expect(viewIdForType('anomaly')).toBeNull()
  })

  it('returns null for an unregistered type', () => {
    expect(viewIdForType('definitely-not-a-type')).toBeNull()
  })
})

describe('notifyTypeMeta', () => {
  it('resolves a registered type', () => {
    expect(notifyTypeMeta('chat')?.label).toBeTruthy()
  })

  it('returns undefined for an unregistered type rather than a partial object', () => {
    expect(notifyTypeMeta('nope')).toBeUndefined()
  })

  // `task` is an alias understood only by viewIdForType — it is not a real
  // entry, so metadata lookup must not invent one.
  it('does not resolve the `task` alias as its own entry', () => {
    expect(notifyTypeMeta('task')).toBeUndefined()
  })
})
