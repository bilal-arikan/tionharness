import { describe, expect, it } from 'vitest'
import type { Goal } from '@/types/goal'
import {
  authorLabel,
  formatMetricValue,
  metricLabel,
  nextStatuses,
  openQuestions,
  scopeSummary,
} from './goalMeta'

const goal = (over: Partial<Goal> = {}): Goal => ({
  id: 'GOL1',
  name: 'x',
  status: 'draft',
  scope: {},
  primary: { metric: 'recipe.avgCostUSD', direction: 'min' },
  guardrails: [],
  policy: { mode: 'propose' },
  createdAt: 0,
  updatedAt: 0,
  history: [],
  ...over,
})

describe('goalMeta', () => {
  it('offers the right status transitions', () => {
    expect(nextStatuses('draft')).toEqual(['active', 'archived'])
    expect(nextStatuses('active')).toEqual(['paused', 'archived'])
    expect(nextStatuses('paused')).toEqual(['active', 'archived'])
    expect(nextStatuses('archived')).toEqual(['draft'])
  })

  it('formats metric values by unit', () => {
    expect(formatMetricValue(0.5, 'usd')).toBe('$0.500')
    expect(formatMetricValue(2.5, 'usd')).toBe('$2.50')
    expect(formatMetricValue(0.905, 'ratio')).toBe('90.5%')
    expect(formatMetricValue(90, 'sec')).toBe('2 dk')
    expect(formatMetricValue(7200, 'sec')).toBe('2.0 sa')
    expect(formatMetricValue(1500, 'tokens')).toBe('1.5k tok')
    expect(formatMetricValue(null, 'usd')).toBe('—')
  })

  it('summarizes scope and counts open questions', () => {
    expect(scopeSummary(goal())).toBe('tüm workspace')
    expect(scopeSummary(goal({ scope: { recipes: ['a'], tags: ['x', 'y'] } }))).toBe(
      '1 reçete · 2 etiket',
    )
    expect(openQuestions(goal({ questions: ['a', ' ', ''] }))).toBe(1)
  })

  it('labels authors and metrics', () => {
    expect(authorLabel('user')).toBe('Sen')
    expect(authorLabel('agent:goal-writer')).toBe('Hedef yazıcı')
    expect(authorLabel('agent:other')).toBe('other')
    expect(
      metricLabel('k', [
        { key: 'k', label: 'L', unit: 'usd', defaultDirection: 'min', source: 's' },
      ]),
    ).toBe('L')
    expect(metricLabel('missing', [])).toBe('missing')
  })
})
