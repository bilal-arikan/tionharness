import { describe, expect, it } from 'vitest'
import type { Goal, GoalMetricDef } from '@/types/goal'
import { fromFormState, parseNum, toFormState, validateForm } from './goalForm'

const catalog: GoalMetricDef[] = [
  { key: 'recipe.avgCostUSD', label: 'Maliyet', unit: 'usd', defaultDirection: 'min', source: 'r' },
  {
    key: 'recipe.successRate',
    label: 'Başarı',
    unit: 'ratio',
    defaultDirection: 'max',
    source: 'r',
  },
  {
    key: 'judge.rubricScore',
    label: 'Rubrik',
    unit: 'score',
    defaultDirection: 'max',
    source: 'j',
  },
]

const base: Goal = {
  id: 'GOL1',
  name: 'Ucuz inceleme',
  rawText: 'incelemeler pahalı',
  status: 'draft',
  kind: 'metric',
  priority: 2,
  scope: { recipes: ['code-review'] },
  primary: { metric: 'recipe.avgCostUSD', direction: 'min', target: 0.5 },
  guardrails: [{ metric: 'recipe.successRate', min: 0.9, max: null }],
  policy: { mode: 'propose', cooldownHours: 72, minRuns: 5 },
  questions: ['Hangi ajan?'],
  createdAt: 1,
  updatedAt: 1,
  history: [{ at: 1, by: 'agent:goal-writer' }],
}

describe('goalForm', () => {
  it('round-trips a goal through the form state', () => {
    const f = toFormState(base)
    expect(f.primaryTarget).toBe('0.5')
    expect(f.guardrails).toEqual([{ metric: 'recipe.successRate', min: '0.9', max: '' }])
    expect(f.questions).toBe('Hangi ajan?')
    const g = fromFormState(base, f)
    expect(g.primary).toEqual(base.primary)
    expect(g.guardrails).toEqual([{ metric: 'recipe.successRate', min: 0.9, max: null }])
    expect(g.scope.recipes).toEqual(['code-review'])
    expect(g.policy).toEqual({
      mode: 'propose',
      autoApplySurfaces: [],
      cooldownHours: 72,
      minRuns: 5,
    })
    expect(g.questions).toEqual(['Hangi ajan?'])
    // Server-owned fields ride along untouched.
    expect(g.id).toBe('GOL1')
    expect(g.rawText).toBe('incelemeler pahalı')
    expect(g.history).toBe(base.history)
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

  it('auto mode keeps its surfaces, other modes clear them', () => {
    const f = toFormState(base)
    f.mode = 'auto'
    f.autoApplySurfaces = ['thinkingLevel']
    expect(fromFormState(base, f).policy.autoApplySurfaces).toEqual(['thinkingLevel'])
    f.mode = 'off'
    expect(fromFormState(base, f).policy.autoApplySurfaces).toEqual([])
  })

  it('validates like the server', () => {
    const ok = toFormState(base)
    expect(validateForm(ok, catalog)).toBeNull()
    expect(validateForm({ ...ok, name: ' ' }, catalog)).toMatch(/Ad zorunlu/)
    expect(validateForm({ ...ok, primaryMetric: 'nope' }, catalog)).toMatch(/katalogda yok/)
    expect(validateForm({ ...ok, primaryTarget: 'x' }, catalog)).toMatch(/sayı olmalı/)
    expect(
      validateForm({ ...ok, primaryMetric: 'judge.rubricScore', rubric: '' }, catalog),
    ).toMatch(/rubrik/i)
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
    expect(validateForm({ ...ok, mode: 'auto', autoApplySurfaces: [] }, catalog)).toMatch(
      /tersinir/,
    )
    expect(validateForm({ ...ok, minRuns: '-1' }, catalog)).toMatch(/negatif/)
  })
})
