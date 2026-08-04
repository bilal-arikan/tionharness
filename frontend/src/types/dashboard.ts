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
export interface DashboardSummary {
  text: string
  tokens: number
  elided: number
  elidedUnit?: string
  handles: ViewHandle[] | null
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
}
