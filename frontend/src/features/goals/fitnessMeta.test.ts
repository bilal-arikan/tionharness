import { describe, expect, it } from 'vitest'
import type { GoalFitness, MetricValue } from '@/types/evolution'
import { changeLabel, deltaLabel, primaryVerdict, sinceFor } from './fitnessMeta'

const mv = (value: number | null): MetricValue => ({
  metric: 'm',
  value,
  n: 1,
  unit: 'usd',
  available: true,
})

describe('fitnessMeta', () => {
  it('maps window presets to since', () => {
    const now = 1_000_000 * 1000
    expect(sinceFor('7d', now)).toBe(1_000_000 - 7 * 86400)
    expect(sinceFor('all', now)).toBe(0)
    expect(sinceFor('bogus', now)).toBe(1_000_000 - 30 * 86400)
  })

  it('labels changes', () => {
    expect(
      changeLabel({ surface: 'agent', entity: 'AGT1', field: 'model', before: 'a', after: 'b' }),
    ).toBe('Ajan AGT1 · model: a → b')
    expect(
      changeLabel({ surface: 'automation', entity: 'AUT1', field: 'added', after: 'nightly' }),
    ).toBe('Otomasyon AUT1 eklendi (nightly)')
    expect(
      changeLabel({ surface: 'recipe', entity: 'r', field: 'version', before: '', after: '2' }),
    ).toBe('Reçete r · version: — → 2')
  })

  it('judges the primary and deltas by direction', () => {
    const base: GoalFitness = {
      goalId: 'G',
      since: 0,
      now: 0,
      sessions: 1,
      primary: mv(2),
      guardrails: [],
      direction: 'min',
      onTarget: false,
      bySnapshot: [],
    }
    expect(primaryVerdict(base)).toBe('off')
    expect(primaryVerdict({ ...base, onTarget: true })).toBe('ok')
    expect(primaryVerdict({ ...base, primary: mv(null) })).toBe('none')
    expect(deltaLabel(mv(1), mv(2), 'min')).toEqual({ text: '▼ (-50%)', good: true })
    expect(deltaLabel(mv(3), mv(2), 'min').good).toBe(false)
    expect(deltaLabel(mv(3), mv(2), 'max').good).toBe(true)
    expect(deltaLabel(mv(2), mv(2), 'max')).toEqual({ text: '±0', good: null })
    expect(deltaLabel(mv(2), undefined, 'max').text).toBe('')
  })
})
