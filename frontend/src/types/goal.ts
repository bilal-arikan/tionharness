// Evolution goals (_Docs/83 §4.1), mirroring internal/db/models_goal.go and
// the catalog in internal/goals.

export type GoalStatus = 'draft' | 'active' | 'paused' | 'archived'
export type GoalDirection = 'min' | 'max'
export type GoalPolicyMode = 'propose' | 'off'
export type GoalAuthor = 'user' | 'agent:goal-writer' | 'seed' | string

interface GoalScope {
  recipes?: string[]
  agents?: string[]
  automations?: string[]
  tags?: string[]
}

interface GoalMetric {
  metric: string
  direction: GoalDirection
  target?: number | null
}

export interface GoalGuardrail {
  metric: string
  min?: number | null
  max?: number | null
}

interface GoalPolicy {
  mode: GoalPolicyMode
  cooldownHours?: number
  minRuns?: number
}

interface GoalRevision {
  at: number
  by: GoalAuthor
  note?: string
  fields?: string[]
}

export interface Goal {
  id: string
  name: string
  description?: string
  // The user's original statement, verbatim (writer path only).
  rawText?: string
  status: GoalStatus
  scope: GoalScope
  primary: GoalMetric
  guardrails: GoalGuardrail[]
  policy: GoalPolicy
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
  unit: 'usd' | 'sec' | 'tokens' | 'count' | 'ratio' | string
  defaultDirection: GoalDirection
  source: string
  scopes?: string[]
}

interface GoalCandidate {
  id: string
  name: string
}

interface GoalScopeCandidates {
  recipes: GoalCandidate[]
  agents: GoalCandidate[]
  automations: GoalCandidate[]
  tags: string[]
}

export interface GoalCatalog {
  metrics: GoalMetricDef[]
  candidates: GoalScopeCandidates
}

export interface GoalIntakeResult {
  goal: Goal
  created: boolean
  // Recorded writer exchange: open it in Chat to continue.
  sessionId?: string
}
