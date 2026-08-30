import { describe, expect, it } from 'vitest'
import {
  ALL_SESSION_CHIPS,
  normalizeChipsOff,
  sessionChipKey,
  sessionMatchesChips,
  SUBAGENT_CHIP,
} from './sessionKindMeta'

describe('sessionChipKey', () => {
  it('prefers category over executionType and legacy kind', () => {
    expect(sessionChipKey({ kind: 'chat', executionType: 'worker', category: 'subagent' })).toBe(
      SUBAGENT_CHIP,
    )
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

  it('filters subagents independently from chat and worker categories', () => {
    const selected = new Set(ALL_SESSION_CHIPS.filter((key) => key !== SUBAGENT_CHIP))
    expect(
      sessionMatchesChips(
        {
          kind: 'chat',
          category: 'subagent',
          executionType: 'subagent',
          isWorker: false,
          isArchived: false,
        },
        selected,
      ),
    ).toBe(false)
  })
})
