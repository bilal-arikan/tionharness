import { describe, expect, it } from 'vitest'
import type { Trajectory, TrajectoryNode } from '@/types/trajectory'
import {
  layoutTrajectory,
  phaseSummary,
  trajectoryProgress,
  UNASSIGNED_COL,
} from './trajectoryLayout'

function node(
  partial: Partial<TrajectoryNode> & { id: string; kind: TrajectoryNode['kind'] },
): TrajectoryNode {
  return { origin: 'observed', lane: 0, state: 'pending', ...partial }
}

function fixture(): Trajectory {
  return {
    id: 'RTA1',
    rootSessionId: 'SES1',
    templateRef: 'plan-dev@3',
    revision: 5,
    status: 'running',
    createdAt: 1,
    updatedAt: 2,
    nodes: [
      node({ id: 'p:plan', kind: 'phase', origin: 'declared', state: 'done', profile: 'planner' }),
      node({
        id: 'p:code',
        kind: 'phase',
        origin: 'declared',
        label: 'Kod',
        state: 'active',
        profile: 'coder',
      }),
      node({ id: 'p:review', kind: 'phase', origin: 'declared', state: 'pending', optional: true }),
      node({
        id: 's:SES1',
        kind: 'session',
        refKind: 'session',
        refId: 'SES1',
        lane: 0,
        state: 'active',
      }),
      node({
        id: 's:SES2',
        kind: 'session',
        refKind: 'session',
        refId: 'SES2',
        phaseId: 'p:plan',
        lane: 1,
        state: 'done',
      }),
      node({
        id: 's:SES3',
        kind: 'session',
        refKind: 'session',
        refId: 'SES3',
        phaseId: 'p:code',
        lane: 2,
        state: 'active',
      }),
      node({
        id: 'g:ASK1',
        kind: 'gate',
        refKind: 'ask',
        refId: 'ASK1',
        phaseId: 'p:code',
        lane: 2,
        state: 'active',
      }),
      node({
        id: 'a:docs',
        kind: 'automation',
        origin: 'declared',
        refKind: 'automation',
        refId: 'docs',
        state: 'ghost',
      }),
    ],
    edges: [
      { from: 'p:plan', to: 'p:code', kind: 'next', origin: 'declared' },
      { from: 's:SES1', to: 's:SES2', kind: 'spawned', origin: 'observed' },
      { from: 's:SES2', to: 's:SES1', kind: 'reported', origin: 'observed' },
      { from: 's:SES3', to: 'g:ASK1', kind: 'blocked_by', origin: 'observed' },
    ],
  }
}

describe('layoutTrajectory', () => {
  it('places phases as columns and nodes into (column, lane) cells', () => {
    const l = layoutTrajectory(fixture())
    expect(l.columns.map((c) => c.id)).toEqual(['p:plan', 'p:code', 'p:review', UNASSIGNED_COL])
    expect(l.columns[1].label).toBe('Kod')
    expect(l.columns[2].optional).toBe(true)
    expect(l.activeCol).toBe(1)
    expect(l.root?.id).toBe('s:SES1')
    expect(l.rootToCol).toBe(1)
    expect(l.lanes).toEqual([0, 1, 2])
    const byId = new Map(l.nodes.map((n) => [n.node.id, n]))
    expect(byId.get('s:SES2')).toMatchObject({ col: 0, lane: 1, slot: 0 })
    expect(byId.get('s:SES3')).toMatchObject({ col: 1, lane: 2, slot: 0 })
    // The gate shares the asker's cell and takes the next slot.
    expect(byId.get('g:ASK1')).toMatchObject({ col: 1, lane: 2, slot: 1 })
    // A trajectory-wide ghost watcher lands in the unassigned column.
    expect(byId.get('a:docs')).toMatchObject({ col: 3, ghost: true })
    expect(l.ghosts).toBe(1)
    // next edges are implied by the columns and dropped.
    expect(l.edges.map((e) => e.kind)).toEqual(['spawned', 'reported', 'blocked_by'])
  })

  it('renders a single "faz yok" column for an agent-planned graph without phases', () => {
    const t = fixture()
    t.nodes = t.nodes.filter(
      (n) => n.kind !== 'phase' && n.kind !== 'gate' && n.kind !== 'automation',
    )
    t.nodes.forEach((n) => (n.phaseId = undefined))
    const l = layoutTrajectory(t)
    expect(l.columns).toHaveLength(1)
    expect(l.columns[0].label).toBe('faz yok')
    expect(l.activeCol).toBe(-1)
    expect(l.rootToCol).toBe(0)
  })

  it('summarises phases and progress', () => {
    const t = fixture()
    expect(phaseSummary(t)).toBe('plan ✓ → Kod ● → review ○')
    expect(trajectoryProgress(t)).toEqual({ total: 3, done: 1, active: 'Kod' })
    expect(phaseSummary({ nodes: [] })).toBe('')
  })
})
