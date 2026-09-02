// Per-trajectory layout (Rota F1b): x = phase column, y = lane. Pure: the same
// graph always yields the same grid, so a live update never reshuffles what the
// user is looking at (lanes are assigned once by the backend and never change).
import type {
  Trajectory,
  TrajectoryEdge,
  TrajectoryNode,
  TrajectoryNodeState,
} from '@/types/trajectory'

export interface TrajColumn {
  // Phase node id ("p:plan") or UNASSIGNED_COL for nodes bound to no phase.
  id: string
  label: string
  state: TrajectoryNodeState | 'none'
  profile?: string
  optional?: boolean
  gate?: { kind: string; value?: string }
  index: number
  phase?: TrajectoryNode
}

export interface TrajPlaced {
  node: TrajectoryNode
  col: number
  // Order among the nodes sharing the same (column, lane) cell.
  slot: number
  lane: number
  ghost: boolean
}

export interface TrajLayout {
  columns: TrajColumn[]
  // Distinct lanes, ascending (0 = the root session's lane).
  lanes: number[]
  nodes: TrajPlaced[]
  edges: TrajectoryEdge[]
  // The root session node (lane 0), drawn as a band across the columns it has
  // reached rather than as a cell — it spans the whole plan.
  root?: TrajectoryNode
  // Column index the root band extends to (the active column, else the last
  // column anything was bound to, else 0).
  rootToCol: number
  ghosts: number
  activeCol: number
}

export const UNASSIGNED_COL = 'unassigned'

export function phaseGlyph(state: TrajectoryNodeState | 'none'): string {
  switch (state) {
    case 'active':
      return '●'
    case 'done':
      return '✓'
    case 'failed':
      return '✗'
    case 'skipped':
      return '↷'
    case 'ghost':
      return '◌'
    case 'none':
      return ''
    default:
      return '○'
  }
}

// phaseId strips the "p:" namespace for display.
export function phaseId(nodeId: string): string {
  return nodeId.startsWith('p:') ? nodeId.slice(2) : nodeId
}

export function layoutTrajectory(t: Trajectory): TrajLayout {
  const phases = t.nodes.filter((n) => n.kind === 'phase')
  const columns: TrajColumn[] = phases.map((p, i) => ({
    id: p.id,
    label: p.label || phaseId(p.id),
    state: p.state,
    profile: p.profile,
    optional: p.optional,
    gate: p.gate ?? undefined,
    index: i,
    phase: p,
  }))
  const colIndex = new Map(columns.map((c) => [c.id, c.index]))

  const others = t.nodes.filter((n) => n.kind !== 'phase')
  const root = others.find((n) => n.kind === 'session' && n.lane === 0)
  const needsUnassigned = others.some((n) => n !== root && (!n.phaseId || !colIndex.has(n.phaseId)))
  if (needsUnassigned || columns.length === 0) {
    columns.push({
      id: UNASSIGNED_COL,
      label: columns.length === 0 ? 'faz yok' : 'fazsız',
      state: 'none',
      index: columns.length,
    })
    colIndex.set(UNASSIGNED_COL, columns.length - 1)
  }

  const cellCount = new Map<string, number>()
  const nodes: TrajPlaced[] = []
  const laneSet = new Set<number>([0])
  let maxBoundCol = 0
  for (const n of others) {
    if (n === root) continue
    const col =
      n.phaseId && colIndex.has(n.phaseId)
        ? colIndex.get(n.phaseId)!
        : colIndex.get(UNASSIGNED_COL)!
    const key = `${col}:${n.lane}`
    const slot = cellCount.get(key) ?? 0
    cellCount.set(key, slot + 1)
    laneSet.add(n.lane)
    if (n.state !== 'ghost') maxBoundCol = Math.max(maxBoundCol, col)
    nodes.push({ node: n, col, slot, lane: n.lane, ghost: n.state === 'ghost' })
  }
  const lanes = [...laneSet].sort((a, b) => a - b)
  const activeIdx = columns.findIndex((c) => c.state === 'active')
  const activeCol = activeIdx >= 0 ? activeIdx : -1
  const rootToCol = activeCol >= 0 ? activeCol : maxBoundCol
  return {
    columns,
    lanes,
    nodes,
    edges: t.edges.filter((e) => e.kind !== 'next'),
    root,
    rootToCol,
    ghosts: nodes.filter((n) => n.ghost).length,
    activeCol,
  }
}

// phaseSummary renders the one-line strip used by the chat header and lists:
// "plan ✓ → kod ● → inceleme ○".
export function phaseSummary(t: Pick<Trajectory, 'nodes'>): string {
  const phases = t.nodes.filter((n) => n.kind === 'phase')
  if (phases.length === 0) return ''
  return phases.map((p) => `${p.label || phaseId(p.id)} ${phaseGlyph(p.state)}`).join(' → ')
}

// trajectoryProgress counts declared phases by outcome (for badges).
export function trajectoryProgress(t: Pick<Trajectory, 'nodes'>): {
  total: number
  done: number
  active?: string
} {
  const phases = t.nodes.filter((n) => n.kind === 'phase')
  const done = phases.filter((p) => p.state === 'done' || p.state === 'skipped').length
  const active = phases.find((p) => p.state === 'active')
  return {
    total: phases.length,
    done,
    active: active ? active.label || phaseId(active.id) : undefined,
  }
}
