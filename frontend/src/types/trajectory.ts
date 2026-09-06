// Trajectory ("Rota") payloads, mirroring internal/db/models_trajectory.go.
// A trajectory is the declared-plus-observed graph of one root session: the
// phases it announced and the sessions / flow runs / automation fires that then
// happened under them (_Docs/77 R4).

export type TrajectoryStatus = 'planned' | 'running' | 'waiting' | 'done' | 'failed' | 'abandoned'

type TrajectoryNodeKind = 'phase' | 'session' | 'automation' | 'flowrun' | 'gate' | 'optimizer'

type TrajectoryNodeOrigin = 'declared' | 'observed'

export type TrajectoryNodeState = 'pending' | 'active' | 'done' | 'failed' | 'skipped' | 'ghost'

type TrajectoryEdgeKind =
  'next' | 'spawned' | 'reported' | 'fired' | 'feeds' | 'blocked_by' | 'forked_from'

interface TrajectoryGate {
  kind: 'artifact' | 'verdict' | 'human' | 'schema'
  value?: string
}

export interface TrajectoryNode {
  id: string
  kind: TrajectoryNodeKind
  origin: TrajectoryNodeOrigin
  label?: string
  // Bridge to the View layer: refKind/refId = view.Ref{Kind, ID}.
  refKind?: string
  refId?: string
  phaseId?: string
  // Drawing row, assigned once and never changed (0 = the root session's lane).
  lane: number
  state: TrajectoryNodeState
  profile?: string
  // Declared phase the trajectory may finish without.
  optional?: boolean
  gate?: TrajectoryGate | null
  reason?: string
  startMs?: number
  endMs?: number
}

export interface TrajectoryEdge {
  from: string
  to: string
  kind: TrajectoryEdgeKind
  origin: TrajectoryNodeOrigin
}

// Deterministic end-of-run digest (Rota F3, db.TrajectorySummary).
interface PhaseStat {
  sessions: number
  failed: number
  durationSec: number
}

interface TrajectorySummary {
  at: number
  durationSec: number
  tokens: number
  costUsd: number
  priced: boolean
  sessions: number
  failedSessions: number
  flowRuns: number
  failedRuns: number
  phases: number
  phasesDone: number
  ghostPhases?: string[]
  unannounced: number
  watchers: number
  unfiredWatchers?: string[]
  gates: number
  gateWaitSec: number
  perPhase?: Record<string, PhaseStat>
}

// Per-recipe-version rollup of the index (agent.RecipeStats).
export interface RecipeStats {
  templateRef: string
  slug: string
  version?: string
  runs: number
  done: number
  failed: number
  abandoned: number
  live: number
  summarized: number
  avgDurationSec: number
  avgTokens: number
  avgCostUsd: number
  priced: boolean
  avgSessions: number
  failedSessions: number
  unannounced: number
  gateWaitSec: number
  unfiredWatchers?: Record<string, number>
  ghostPhases?: Record<string, number>
  lastAt: number
  latestId?: string
}

// Canvas actions (Rota F5).
export interface PlanTrajectoryReq {
  phases: {
    id: string
    label?: string
    profile?: string
    optional?: boolean
    gate?: TrajectoryGate
  }[]
  expectedRev: number
}

export interface SetPhaseReq {
  id: string
  state: 'active' | 'done' | 'skipped' | 'failed'
  reason?: string
  force?: boolean
  expectedRev: number
}

export interface FinishTrajectoryReq {
  status: 'done' | 'failed'
  reason?: string
  expectedRev: number
}

// Recipe optimizer (Rota F4).
export interface OptimizerResult {
  slug: string
  trigger: string
  ran: boolean
  skipped?: string
  proposals: { id: string; title: string }[]
  dropped: number
  applied?: number
}

export interface OptimizerStateRow {
  slug: string
  ran: boolean
  state: {
    lastAt: number
    runsSeen: number
    trigger?: string
    proposals: number
    skipped?: string
  }
}

export interface Trajectory {
  id: string
  rootSessionId: string
  templateRef?: string
  revision: number
  status: TrajectoryStatus
  nodes: TrajectoryNode[]
  edges: TrajectoryEdge[]
  meta?: Record<string, string>
  summary?: TrajectorySummary
  createdAt: number
  updatedAt: number
}

// Listing-sized summary (trajectories/index.json row).
export interface TrajectoryIndexEntry {
  id: string
  rootSessionId: string
  templateRef?: string
  status: TrajectoryStatus
  revision: number
  nodeCount: number
  summary?: TrajectorySummary
  createdAt: number
  updatedAt: number
}
