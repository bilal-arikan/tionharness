// Workspace dashboard types — mirrors internal/api/dashboard.go.
//
// The series are aggregated on the backend: the browser gets counts, never the
// underlying sessions/tasks/runs. That is the same economy the projection layer
// applies to agents (_Docs/66) — a dashboard that downloads the whole workspace
// to count it defeats its own purpose.

import type { ViewHandle } from './view'

export interface DaySeriesPoint {
  day: string // YYYY-MM-DD
  value: number
}

export interface CommitActivityDay {
  day: string // YYYY-MM-DD in the server's local timezone
  value: number
}

export interface CommitActivity {
  weeks: number
  commitsByDay: CommitActivityDay[]
  // Older servers return an all-zero series for a non-repository. Newer servers
  // can set this to false so the UI can explain that state separately.
  isGitRepo?: boolean
}

export interface NamedCount {
  name: string
  count: number
}

export interface DashboardCounters {
  agents: number
  sessions: number
  sessionsActive: number
  sessionsStuck: number
  sessionsArchived: number
  tasks: number
  tasksOpen: number
  runs: number
  runsRunning: number
  runsWaiting: number
  runsFailed: number
}

// The workspace projection, byte-identical to what an agent receives from
// get_view{kind:"workspace"}.
interface DashboardSummary {
  text: string
  tokens: number
  elided: number
  elidedUnit?: string
  handles: ViewHandle[] | null
}

// A daily USD series — the float sibling of DaySeriesPoint.
interface DayCostPoint {
  day: string
  value: number
}

// One agent's priced spend, for the "costliest agents" ranking.
export interface NamedCost {
  name: string
  cost: number
}

// The headline money summary. Every figure is priced on the backend by the same
// billing rollup the Budget screen uses, so the two can never disagree.
export interface CostBlock {
  today: number
  month: number // calendar month-to-date
  estimated: boolean // any spend priced via an equivalent-API estimate
  burnRate: number // average USD/day over the window
  projectedMonth: number
  coolingWaste: number // avoidable cache-cooling overpay over the window
}

// A metric's current window vs the previous one of equal length. pct is null
// when there is no baseline (prev == 0) — a change off zero is undefined.
export interface DeltaStat {
  curr: number
  prev: number
  pct: number | null
}

interface DashboardDeltas {
  sessions: DeltaStat
  runs: DeltaStat
  tokens: DeltaStat
  cost: DeltaStat
}

// One clickable row of the attention queue. kind + id route the click to the
// session, run, card or schedule it points at.
export interface ActionItem {
  severity: 'danger' | 'warn'
  kind: 'session' | 'run' | 'card' | 'schedule'
  id: string
  label: string
  detail?: string
  age?: string
}

// The completion-side metrics: are things finishing, not just starting.
export interface OutcomeBlock {
  velocityByDay: DaySeriesPoint[]
  cardsDoneWindow: number
  cycleTimeAvgSec: number
  runsClosedWindow: number
  runSuccessRate: number | null // 0..1, null when nothing closed
}

export interface Dashboard {
  asOf: string
  summary: DashboardSummary
  counters: DashboardCounters
  sessionsByDay: DaySeriesPoint[]
  runsByDay: DaySeriesPoint[]
  tokensByDay: DaySeriesPoint[]
  boardByColumn: NamedCount[]
  runsByStatus: NamedCount[]
  sessionsByKind: NamedCount[]
  topAgents: NamedCount[]
  cost: CostBlock
  costByDay: DayCostPoint[]
  topAgentsCost: NamedCost[]
  actions: ActionItem[]
  deltas: DashboardDeltas
  outcomes: OutcomeBlock
}
