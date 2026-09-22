import { describe, expect, it } from 'vitest'
import type { BucketStats, GoalFitness, MetricValue, SnapshotFitness } from '@/types/evolution'
import {
  changeLabel,
  deltaLabel,
  primaryVerdict,
  sinceFor,
  snapshotDelta,
  statValue,
} from './fitnessMeta'

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
      agents: [],
      providerFailures: 0,
      minBucketSessions: 50,
      maxTop3CostShare: 0.4,
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

  describe('version breakdown', () => {
    const dist = { n: 60, mean: 4, median: 1.5, trimmedMean: 2, top3Share: 0.2 }
    const stats = (over: Partial<BucketStats> = {}): BucketStats => ({
      cost: dist,
      toolCalls: 600,
      costPerToolCall: 0.4,
      insufficient: false,
      ...over,
    })
    const bucket = (value: number, st: BucketStats, median = value): SnapshotFitness => ({
      hash: 'h',
      from: 0,
      to: 0,
      sessions: 60,
      current: false,
      primary: { ...mv(value), dist: { ...dist, mean: value, median } },
      guardrails: [],
      stats: st,
      providerFailures: 0,
    })

    it('picks the chosen statistic, falling back to the plain value', () => {
      const m = { ...mv(4), dist }
      expect(statValue(m, 'mean')).toBe(4)
      expect(statValue(m, 'median')).toBe(1.5)
      expect(statValue(m, 'trimmed')).toBe(2)
      expect(statValue(mv(7), 'median')).toBe(7)
      expect(statValue(mv(null), 'median')).toBeNull()
    })

    it('shows "yetersiz veri" instead of a delta when a bucket fails the gate', () => {
      const prev = bucket(2, stats())
      const small = bucket(3.5, stats({ insufficient: true, reasons: ['n=27 < 50'] }))
      const d = snapshotDelta(small, prev, 'min', 'mean')
      expect(d.gated).toBe(true)
      expect(d.text).toBe('yetersiz veri')
      expect(d.good).toBeNull()
      expect(d.reasons).toEqual(['bu sürüm: n=27 < 50'])
      // The previous bucket failing the gate blocks the comparison too.
      const d2 = snapshotDelta(bucket(3.5, stats()), bucket(2, small.stats), 'min', 'mean')
      expect(d2.gated).toBe(true)
      expect(d2.reasons).toEqual(['önceki: n=27 < 50'])
    })

    it('computes the delta on the chosen statistic when both buckets pass', () => {
      const prev = bucket(2, stats(), 1)
      const cur = bucket(3.5, stats(), 1)
      expect(snapshotDelta(cur, prev, 'min', 'mean')).toMatchObject({
        text: '▲ (+75%)',
        good: false,
        gated: false,
      })
      expect(snapshotDelta(cur, prev, 'min', 'median').text).toBe('±0')
      expect(snapshotDelta(cur, undefined, 'min', 'median').text).toBe('')
    })
  })
})
