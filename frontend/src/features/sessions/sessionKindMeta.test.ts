import { describe, expect, it } from 'vitest'
import {
  ALL_SESSION_CHIPS,
  normalizeChipsOff,
  RUNNING_CHIP,
  sessionChipKey,
  sessionLiveScope,
  sessionMatchesChips,
  SUBAGENT_CHIP,
} from './sessionKindMeta'

describe('sessionChipKey', () => {
  it('prefers category over executionType and legacy kind', () => {
    expect(
      sessionChipKey({ kind: 'chat', executionType: 'interactive', category: 'subagent' }),
    ).toBe(SUBAGENT_CHIP)
  })

  it('uses executionType before legacy kind when category is absent', () => {
    expect(sessionChipKey({ kind: 'chat', executionType: 'subagent' })).toBe(SUBAGENT_CHIP)
  })

  it('keeps legacy kind fallback', () => {
    expect(sessionChipKey({ kind: 'spawned' })).toBe('spawned')
  })

  it('leaves newly added subagent chip visible for old persisted filters', () => {
    expect(normalizeChipsOff(JSON.stringify(['chat', 'removed-chip']))).toEqual(['chat'])
  })

  it('filters subagents independently from the chat category', () => {
    const selected = new Set(ALL_SESSION_CHIPS.filter((key) => key !== SUBAGENT_CHIP))
    expect(
      sessionMatchesChips(
        {
          kind: 'chat',
          category: 'subagent',
          executionType: 'subagent',
          isArchived: false,
        },
        selected,
      ),
    ).toBe(false)
  })
})

describe('sessionLiveScope', () => {
  it('reports a session with its own live turn as running', () => {
    expect(sessionLiveScope({ isRunning: true })).toBe(RUNNING_CHIP)
  })

  it('classifies an idle session as not live', () => {
    expect(sessionLiveScope({ isRunning: false })).toBeNull()
  })
})

describe('sessionMatchesChips live scopes', () => {
  const base = { kind: 'chat', isArchived: false }

  it('hides a running session when the running chip is off', () => {
    const selected = new Set(ALL_SESSION_CHIPS.filter((key) => key !== RUNNING_CHIP))
    expect(sessionMatchesChips({ ...base, isRunning: true }, selected)).toBe(false)
  })

  it('keeps an idle session visible when only the live chip is off', () => {
    const selected = new Set(ALL_SESSION_CHIPS.filter((key) => key !== RUNNING_CHIP))
    expect(sessionMatchesChips({ ...base, isRunning: false }, selected)).toBe(true)
  })

  it('ignores the live axis entirely when the caller has no runtime snapshot', () => {
    const selected = new Set(ALL_SESSION_CHIPS.filter((key) => key !== RUNNING_CHIP))
    expect(sessionMatchesChips(base, selected)).toBe(true)
  })
})
