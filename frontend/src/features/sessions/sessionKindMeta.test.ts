import { describe, expect, it } from 'vitest'
import {
  ALL_SESSION_CHIPS,
  AWAITING_WORKERS_CHIP,
  normalizeChipsOff,
  RUNNING_CHIP,
  sessionChipKey,
  sessionLiveScope,
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

describe('sessionLiveScope', () => {
  it('reports a session with its own live turn as running', () => {
    expect(sessionLiveScope({ isRunning: true, liveWorkerCount: 0 })).toBe(RUNNING_CHIP)
  })

  it('reports an idle session with live workers below it as awaiting workers', () => {
    expect(sessionLiveScope({ isRunning: false, liveWorkerCount: 2 })).toBe(AWAITING_WORKERS_CHIP)
  })

  it('prefers running over awaiting so the two scopes never overlap', () => {
    expect(sessionLiveScope({ isRunning: true, liveWorkerCount: 3 })).toBe(RUNNING_CHIP)
  })

  it('classifies a fully idle session as neither', () => {
    expect(sessionLiveScope({ isRunning: false, liveWorkerCount: 0 })).toBeNull()
  })
})

describe('sessionMatchesChips live scopes', () => {
  const base = { kind: 'chat', isWorker: false, isArchived: false }

  it('hides a running session when the running chip is off', () => {
    const selected = new Set(ALL_SESSION_CHIPS.filter((key) => key !== RUNNING_CHIP))
    expect(sessionMatchesChips({ ...base, isRunning: true, liveWorkerCount: 0 }, selected)).toBe(
      false,
    )
  })

  it('hides a coordinator waiting on workers when that chip is off', () => {
    const selected = new Set(ALL_SESSION_CHIPS.filter((key) => key !== AWAITING_WORKERS_CHIP))
    expect(sessionMatchesChips({ ...base, isRunning: false, liveWorkerCount: 1 }, selected)).toBe(
      false,
    )
  })

  it('keeps an idle session visible when only the live chips are off', () => {
    const selected = new Set(
      ALL_SESSION_CHIPS.filter((key) => key !== RUNNING_CHIP && key !== AWAITING_WORKERS_CHIP),
    )
    expect(sessionMatchesChips({ ...base, isRunning: false, liveWorkerCount: 0 }, selected)).toBe(
      true,
    )
  })

  it('ignores the live axis entirely when the caller has no runtime snapshot', () => {
    const selected = new Set(
      ALL_SESSION_CHIPS.filter((key) => key !== RUNNING_CHIP && key !== AWAITING_WORKERS_CHIP),
    )
    expect(sessionMatchesChips(base, selected)).toBe(true)
  })
})
