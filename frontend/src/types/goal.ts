// Evolution goals (_Docs/83 §4.1), mirroring internal/db/models_goal.go and
// the catalog in internal/goals.

export type GoalStatus = 'draft' | 'active' | 'paused' | 'archived'
export type GoalKind = 'metric' | 'rubric' | 'mixed'
export type GoalDirection = 'min' | 'max'
export type GoalPolicyMode = 'propose' | 'auto' | 'off'
export type GoalAuthor = 'user' | 'agent:goal-writer' | string

export interface GoalScope {
  recipes?: string[]
  agents?: string[]
  automations?: string[]
  tags?: string[]
}

export interface GoalMetric {
  metric: string
  direction: GoalDirection
  target?: number | null
}

export interface GoalGuardrail {
  metric: string
  min?: number | null
  max?: number | null
}

export interface GoalPolicy {
  mode: GoalPolicyMode
  autoApplySurfaces?: string[]
  cooldownHours?: number
  minRuns?: number
}

export interface GoalRevision {
  at: number
  by: GoalAuthor
  note?: string
  fields?: string[]
}

export interface Goal {
  id: string
  name: string
  summary?: string
  description?: string
  // The user's original statement, verbatim.
  rawText?: string
  status: GoalStatus
  kind?: GoalKind
  priority?: number
  scope: GoalScope
  primary: GoalMetric
  guardrails: GoalGuardrail[]
  rubric?: string
  policy: GoalPolicy
  questions?: string[]
  notes?: string
  createdBy?: GoalAuthor
  createdAt: number
  updatedAt: number
  history: GoalRevision[]
}

// One catalog metric (internal/goals/catalog.go).
export interface GoalMetricDef {
  key: string
  label: string
  hint?: string
  unit: 'usd' | 'sec' | 'tokens' | 'count' | 'ratio' | 'score' | string
  defaultDirection: GoalDirection
  source: string
  scopes?: string[]
}

export interface GoalCandidate {
  id: string
  name: string
}

export interface GoalScopeCandidates {
  recipes: GoalCandidate[]
  agents: GoalCandidate[]
  automations: GoalCandidate[]
  tags: string[]
}

export interface GoalCatalog {
  metrics: GoalMetricDef[]
  autoApplySurfaces: string[]
  candidates: GoalScopeCandidates
}

export interface GoalIntakeResult {
  goal: Goal
  created: boolean
}
