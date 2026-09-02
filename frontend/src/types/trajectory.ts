// Trajectory ("Rota") payloads, mirroring internal/db/models_trajectory.go.
// A trajectory is the declared-plus-observed graph of one root session: the
// phases it announced and the sessions / flow runs / automation fires that then
// happened under them (_Docs/77 R4).

export type TrajectoryStatus = 'planned' | 'running' | 'waiting' | 'done' | 'failed' | 'abandoned'

export type TrajectoryNodeKind =
  'phase' | 'session' | 'automation' | 'flowrun' | 'gate' | 'optimizer'

export type TrajectoryNodeOrigin = 'declared' | 'observed'

export type TrajectoryNodeState = 'pending' | 'active' | 'done' | 'failed' | 'skipped' | 'ghost'

export type TrajectoryEdgeKind =
  'next' | 'spawned' | 'reported' | 'fired' | 'feeds' | 'blocked_by' | 'forked_from'

export interface TrajectoryGate {
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

export interface Trajectory {
  id: string
  rootSessionId: string
  templateRef?: string
  revision: number
  status: TrajectoryStatus
  nodes: TrajectoryNode[]
  edges: TrajectoryEdge[]
  meta?: Record<string, string>
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
  createdAt: number
  updatedAt: number
}
