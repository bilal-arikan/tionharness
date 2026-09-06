import type { InsightFinding } from './insight'

// Evolution fitness + configuration snapshots (_Docs/83 §4.2, E1), mirroring
// internal/goals/fitness.go, snapshot.go and internal/api/goal_fitness.go.

export interface MetricValue {
  metric: string
  value: number | null
  n: number
  unit: string
  available: boolean
  note?: string
}

export interface GuardrailStatus extends MetricValue {
  min?: number | null
  max?: number | null
  ok: boolean
  violated: boolean
}

export interface SnapshotChange {
  surface:
    | 'agent'
    | 'tools'
    | 'recipe'
    | 'automation'
    | 'schedule'
    | 'prompt'
    | 'settings'
    | 'model'
    | string
  entity?: string
  field?: string
  before?: string
  after?: string
}

export interface SnapshotFitness {
  hash: string
  from: number
  to: number
  sessions: number
  current: boolean
  primary: MetricValue
  guardrails: GuardrailStatus[]
  changes?: SnapshotChange[]
}

export interface GoalFitness {
  goalId: string
  since: number
  now: number
  sessions: number
  primary: MetricValue
  guardrails: GuardrailStatus[]
  direction: 'min' | 'max'
  target?: number | null
  onTarget?: boolean | null
  bySnapshot: SnapshotFitness[]
}

interface SnapshotRow {
  hash: string
  firstSeen: number
  lastSeen: number
  prev?: string
  current: boolean
  sessions: number
  changes?: SnapshotChange[]
}

export interface SnapshotList {
  current: string
  unstamped: number
  snapshots: SnapshotRow[]
}

// One evolver pass (internal/agent EvolutionResult).
export interface EvolutionResult {
  goalId: string
  trigger: string
  ran: boolean
  skipped?: string
  lowConfidence?: boolean
  proposals: InsightFinding[]
  dropped: number
  dropReasons?: string[]
  sessions: number
}

// Evolver bookkeeping for one goal (db.EvolutionGoalState).
interface EvolutionGoalState {
  lastAt: number
  sessionsSeen: number
  trigger?: string
  proposals: number
  dropped: number
  skipped?: string
  snapshotHash?: string
  repeats?: Record<string, number>
  escalated?: Record<string, boolean>
}

export interface GoalEvolution {
  goalId: string
  ran: boolean
  state: EvolutionGoalState
  findings: InsightFinding[]
  minRuns: number
  maxProposals: number
}
