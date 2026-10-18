import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { i18next } from '@/i18n'
import { emptyLanes, type LaneState } from '@/shared/lib/laneModel'
import { diffSignals } from './workspaceSignals'

function withFires(s: LaneState, fires: LaneState['fires']): LaneState {
  return { ...s, fires }
}

beforeEach(() => i18next.changeLanguage('tr'))
afterEach(() => i18next.changeLanguage('tr'))

describe('diffSignals', () => {
  it('uses the current language for notifications while retaining event data', async () => {
    const before = emptyLanes()
    const after = withFires(before, [
      {
        seq: 1,
        at: 1,
        automationId: 'AUT1',
        name: 'A user-provided name',
        triggerKind: 'tag',
        outcome: 'fired',
      },
    ])
    await i18next.changeLanguage('en')
    expect(diffSignals(before, after)[0]?.text).toBe('⚡ Automation A user-provided name fired')
    await i18next.changeLanguage('tr')
    expect(diffSignals(before, after)[0]?.text).toBe('⚡ Otomasyon A user-provided name ateşlendi')
  })

  it('toasts new fires only, skipping cooldown noise', () => {
    const a = emptyLanes()
    const b = withFires(a, [
      {
        seq: 1,
        at: 1,
        automationId: 'AUT1',
        name: 'Docs',
        triggerKind: 'tag',
        outcome: 'fired',
        sessionId: 'SES5',
      },
      {
        seq: 2,
        at: 2,
        automationId: 'AUT2',
        triggerKind: 'tag',
        outcome: 'skipped',
        reason: 'cooldown',
      },
      {
        seq: 3,
        at: 3,
        automationId: 'AUT3',
        triggerKind: 'tag',
        outcome: 'skipped',
        reason: 'max_iterations',
      },
    ])
    const out = diffSignals(a, b)
    expect(out.map((o) => o.text)).toEqual([
      '⚡ Otomasyon Docs ateşlendi → SES5',
      '↷ Otomasyon AUT3 atlandı: iterasyon tavanı',
    ])
    // A stuck rule is a warning, not routine news.
    expect(out.map((o) => o.level)).toEqual(['info', 'warning'])
    // Nothing new → nothing said.
    expect(diffSignals(b, b)).toEqual([])
  })

  it('honours the per-type mute', () => {
    const a = emptyLanes()
    const b = withFires(a, [
      { seq: 1, at: 1, automationId: 'AUT1', triggerKind: 'tag', outcome: 'fired' },
    ])
    expect(diffSignals(a, b, (t) => t !== 'automation')).toEqual([])
  })
})
