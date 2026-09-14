// Pure helpers for the goal editor: the form's string-typed draft, its
// conversion back to a Goal payload, and the client-side checks that mirror
// internal/goals/validate.go (the server is still the authority).
import type { Goal, GoalGuardrail, GoalMetricDef } from '@/types/goal'

interface GuardrailDraft {
  metric: string
  min: string
  max: string
}

export interface GoalFormState {
  name: string
  description: string
  primaryMetric: string
  primaryDirection: 'min' | 'max'
  primaryTarget: string
  guardrails: GuardrailDraft[]
  recipes: string[]
  agents: string[]
  automations: string[]
  tags: string[]
  mode: Goal['policy']['mode']
  cooldownHours: string
  minRuns: string
}

const num = (v: number | null | undefined) => (v === null || v === undefined ? '' : String(v))

// emptyGoal is the blank the editor starts from when the user creates a goal
// directly; the server assigns id, timestamps, provenance and history.
export function emptyGoal(): Goal {
  return {
    id: '',
    name: '',
    status: 'draft',
    scope: {},
    primary: { metric: '', direction: 'min' },
    guardrails: [],
    policy: { mode: 'propose' },
    createdAt: 0,
    updatedAt: 0,
    history: [],
  }
}

export function toFormState(g: Goal): GoalFormState {
  return {
    name: g.name,
    description: g.description ?? '',
    primaryMetric: g.primary.metric,
    primaryDirection: g.primary.direction,
    primaryTarget: num(g.primary.target),
    guardrails: g.guardrails.map((r) => ({ metric: r.metric, min: num(r.min), max: num(r.max) })),
    recipes: g.scope.recipes ?? [],
    agents: g.scope.agents ?? [],
    automations: g.scope.automations ?? [],
    tags: g.scope.tags ?? [],
    mode: g.policy.mode,
    cooldownHours: g.policy.cooldownHours ? String(g.policy.cooldownHours) : '',
    minRuns: g.policy.minRuns ? String(g.policy.minRuns) : '',
  }
}

// parseNum turns a form field into a number or null; "" → null, junk → NaN.
export function parseNum(s: string): number | null {
  const t = s.trim().replace(',', '.')
  if (t === '') return null
  return Number(t)
}

// fromFormState rebuilds the goal payload over the stored goal (id, status,
// raw text and history are the server's to keep).
export function fromFormState(base: Goal, f: GoalFormState): Goal {
  const guardrails: GoalGuardrail[] = f.guardrails
    .filter((r) => r.metric.trim() !== '')
    .map((r) => ({
      metric: r.metric.trim(),
      min: parseNum(r.min),
      max: parseNum(r.max),
    }))
  return {
    ...base,
    name: f.name.trim(),
    description: f.description.trim(),
    scope: { recipes: f.recipes, agents: f.agents, automations: f.automations, tags: f.tags },
    primary: {
      metric: f.primaryMetric,
      direction: f.primaryDirection,
      target: parseNum(f.primaryTarget),
    },
    guardrails,
    policy: {
      mode: f.mode,
      cooldownHours: parseNum(f.cooldownHours) ?? 0,
      minRuns: parseNum(f.minRuns) ?? 0,
    },
  }
}

// validateForm returns the first client-side problem, or null.
export function validateForm(f: GoalFormState, catalog: GoalMetricDef[]): string | null {
  if (f.name.trim() === '') return 'Ad zorunlu.'
  if (!f.primaryMetric) return 'Ana metrik seç.'
  if (!catalog.some((m) => m.key === f.primaryMetric)) return 'Ana metrik katalogda yok.'
  if (f.primaryTarget.trim() !== '' && Number.isNaN(parseNum(f.primaryTarget)))
    return 'Hedef değer sayı olmalı.'
  const seen = new Set<string>()
  for (const [i, r] of f.guardrails.entries()) {
    const n = i + 1
    if (r.metric.trim() === '') return `Guardrail ${n}: metrik seç.`
    if (r.metric === f.primaryMetric) return `Guardrail ${n}: ana metrikle aynı olamaz.`
    if (seen.has(r.metric)) return `Guardrail ${n}: aynı metrik iki kez.`
    seen.add(r.metric)
    const min = parseNum(r.min)
    const max = parseNum(r.max)
    if (Number.isNaN(min) || Number.isNaN(max)) return `Guardrail ${n}: sınırlar sayı olmalı.`
    if (min === null && max === null) return `Guardrail ${n}: alt ya da üst sınır gir.`
    if (min !== null && max !== null && min > max) return `Guardrail ${n}: alt sınır üstten büyük.`
  }
  for (const v of [f.cooldownHours, f.minRuns]) {
    const n = parseNum(v)
    if (Number.isNaN(n) || (n !== null && n < 0)) return 'Cooldown ve minimum koşu negatif olamaz.'
  }
  return null
}
