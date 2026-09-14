import { describe, expect, it } from 'vitest'
import type { Goal, GoalMetricDef } from '@/types/goal'
import { emptyGoal, fromFormState, parseNum, toFormState, validateForm } from './goalForm'

const catalog: GoalMetricDef[] = [
  { key: 'recipe.avgCostUSD', label: 'Maliyet', unit: 'usd', defaultDirection: 'min', source: 'r' },
  {
    key: 'recipe.successRate',
    label: 'Başarı',
    unit: 'ratio',
    defaultDirection: 'max',
    source: 'r',
  },
]

const base: Goal = {
  id: 'GOL1',
  name: 'Ucuz inceleme',
  rawText: 'incelemeler pahalı',
  status: 'draft',
  scope: { recipes: ['code-review'] },
  primary: { metric: 'recipe.avgCostUSD', direction: 'min', target: 0.5 },
  guardrails: [{ metric: 'recipe.successRate', min: 0.9, max: null }],
  policy: { mode: 'propose', cooldownHours: 72, minRuns: 5 },
  createdAt: 1,
  updatedAt: 1,
  history: [{ at: 1, by: 'agent:goal-writer' }],
}

describe('goalForm', () => {
  it('round-trips a goal through the form state', () => {
    const f = toFormState(base)
    expect(f.primaryTarget).toBe('0.5')
    expect(f.guardrails).toEqual([{ metric: 'recipe.successRate', min: '0.9', max: '' }])
    const g = fromFormState(base, f)
    expect(g.primary).toEqual(base.primary)
    expect(g.guardrails).toEqual([{ metric: 'recipe.successRate', min: 0.9, max: null }])
    expect(g.scope.recipes).toEqual(['code-review'])
    expect(g.policy).toEqual({ mode: 'propose', cooldownHours: 72, minRuns: 5 })
    // Server-owned fields ride along untouched.
    expect(g.id).toBe('GOL1')
    expect(g.rawText).toBe('incelemeler pahalı')
    expect(g.history).toBe(base.history)
  })

  it('starts a new goal from a blank that fails validation until filled', () => {
    const blank = emptyGoal()
    const f = toFormState(blank)
    expect(validateForm(f, catalog)).toMatch(/Ad zorunlu/)
    f.name = 'Yeni'
    expect(validateForm(f, catalog)).toMatch(/Ana metrik seç/)
    f.primaryMetric = 'recipe.avgCostUSD'
    expect(validateForm(f, catalog)).toBeNull()
    expect(fromFormState(blank, f).policy.mode).toBe('propose')
  })

  it('parses numbers leniently and drops empty guardrail rows', () => {
    expect(parseNum(' 0,75 ')).toBe(0.75)
    expect(parseNum('')).toBeNull()
    expect(parseNum('abc')).toBeNaN()
    const f = toFormState(base)
    f.guardrails.push({ metric: '', min: '1', max: '' })
    f.primaryTarget = ''
    const g = fromFormState(base, f)
    expect(g.guardrails).toHaveLength(1)
    expect(g.primary.target).toBeNull()
  })

  it('validates like the server', () => {
    const ok = toFormState(base)
    expect(validateForm(ok, catalog)).toBeNull()
    expect(validateForm({ ...ok, name: ' ' }, catalog)).toMatch(/Ad zorunlu/)
    expect(validateForm({ ...ok, primaryMetric: 'nope' }, catalog)).toMatch(/katalogda yok/)
    expect(validateForm({ ...ok, primaryTarget: 'x' }, catalog)).toMatch(/sayı olmalı/)
    expect(
      validateForm(
        { ...ok, guardrails: [{ metric: 'recipe.avgCostUSD', min: '1', max: '' }] },
        catalog,
      ),
    ).toMatch(/ana metrikle aynı/)
    expect(
      validateForm(
        { ...ok, guardrails: [{ metric: 'recipe.successRate', min: '', max: '' }] },
        catalog,
      ),
    ).toMatch(/alt ya da üst/)
    expect(
      validateForm(
        { ...ok, guardrails: [{ metric: 'recipe.successRate', min: '2', max: '1' }] },
        catalog,
      ),
    ).toMatch(/üstten büyük/)
    expect(validateForm({ ...ok, minRuns: '-1' }, catalog)).toMatch(/negatif/)
  })
})
