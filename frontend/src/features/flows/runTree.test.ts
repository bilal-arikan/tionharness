import { describe, it, expect } from 'vitest'

import {
  buildRunTreeRows,
  applyChildFrame,
  childProgressFromTree,
  mergeChildProgress,
  runTreeBreadcrumb,
  type ChildProgress,
} from './runTree'
import type { FlowRun, FlowNodeEvent } from '@/types'

const run = (id: string, parentRunId?: string, rootRunId?: string): FlowRun =>
  ({
    id,
    flowId: 'F1',
    parentRunId,
    rootRunId,
    status: 'success',
    input: '',
    state: '',
    output: '',
    error: '',
    createdAt: 0,
    updatedAt: 0,
  }) as FlowRun

const ev = (over: Partial<FlowNodeEvent> = {}): FlowNodeEvent =>
  ({
    phase: 'start',
    nodeId: 'n1',
    type: 'agent',
    title: 'İnceleme',
    index: 1,
    ...over,
  }) as FlowNodeEvent

describe('buildRunTreeRows', () => {
  it('indents each run one level below its parent', () => {
    const rows = buildRunTreeRows([
      run('R1'),
      run('R2', 'R1', 'R1'),
      run('R3', 'R1', 'R1'),
      run('R4', 'R2', 'R1'),
    ])
    expect(rows.map((r) => [r.run.id, r.depth])).toEqual([
      ['R1', 0],
      ['R2', 1],
      ['R3', 1],
      ['R4', 2],
    ])
  })

  it('preserves the backend order instead of re-sorting', () => {
    // The server orders breadth-first by parent links precisely because
    // timestamps cannot separate a parent from the child it launches in the same
    // second. Re-sorting here would reintroduce that bug on the client.
    const rows = buildRunTreeRows([run('R1'), run('R2', 'R1', 'R1'), run('R3', 'R1', 'R1')])
    expect(rows.map((r) => r.run.id)).toEqual(['R1', 'R2', 'R3'])
  })

  it('shows a run with a missing parent unindented rather than dropping it', () => {
    // The backend appends members it cannot reach (parent row deleted) at the end.
    const rows = buildRunTreeRows([run('R1'), run('R9', 'GONE', 'R1')])
    expect(rows).toHaveLength(2)
    expect(rows[1]).toMatchObject({ depth: 0 })
  })
})

describe('applyChildFrame', () => {
  it('keys a child frame by the parent run AND the node that launched it', () => {
    const next = applyChildFrame(
      {},
      { runId: 'R2', parentRunId: 'R1', parentNodeId: 'sub1', ev: ev({ title: 'Test' }) },
    )
    expect(next.R1.sub1).toEqual<ChildProgress>({
      phase: 'start',
      title: 'Test',
      index: 1,
      runId: 'R2',
    })
  })

  it('does not let two runs collide on the same node id', () => {
    // Node ids are unique only WITHIN a graph. The root and a child can each own
    // an "n1"; a flat node-id key would paint one child's progress onto the
    // other's canvas.
    let map = applyChildFrame(
      {},
      { runId: 'R2', parentRunId: 'R1', parentNodeId: 'n1', ev: ev({ title: 'A' }) },
    )
    map = applyChildFrame(map, {
      runId: 'R3',
      parentRunId: 'R2',
      parentNodeId: 'n1',
      ev: ev({ title: 'B' }),
    })
    expect(map.R1.n1.title).toBe('A')
    expect(map.R2.n1.title).toBe('B')
  })

  it('ignores a frame with no parent linkage, returning the map untouched', () => {
    // The tree root's own frames arrive on the same subscription; they belong to
    // the canvas already being drawn, not to a subflow node on it.
    const prev = {
      R1: { sub1: { phase: 'done', title: 'x', index: 1, runId: 'R2' } as ChildProgress },
    }
    expect(applyChildFrame(prev, { runId: 'R1', ev: ev() })).toBe(prev)
    // A run_flow child knows its parent run but hangs off no node — there is
    // nowhere to roll it up to, so it must not guess a node.
    expect(applyChildFrame(prev, { runId: 'R5', parentRunId: 'R1', ev: ev() })).toBe(prev)
  })

  it('lets a later frame replace an earlier one for the same node', () => {
    const first = applyChildFrame(
      {},
      { runId: 'R2', parentRunId: 'R1', parentNodeId: 'sub1', ev: ev({ title: 'A', index: 1 }) },
    )
    const second = applyChildFrame(first, {
      runId: 'R2',
      parentRunId: 'R1',
      parentNodeId: 'sub1',
      ev: ev({ phase: 'done', title: 'B', index: 2 }),
    })
    expect(second.R1.sub1).toMatchObject({ phase: 'done', title: 'B', index: 2 })
  })

  it('keeps two sibling subflow nodes of the same parent independent', () => {
    let map = applyChildFrame(
      {},
      { runId: 'R2', parentRunId: 'R1', parentNodeId: 'sub1', ev: ev({ title: 'A' }) },
    )
    map = applyChildFrame(map, {
      runId: 'R3',
      parentRunId: 'R1',
      parentNodeId: 'sub2',
      ev: ev({ title: 'B' }),
    })
    expect(map.R1.sub1.runId).toBe('R2')
    expect(map.R1.sub2.runId).toBe('R3')
  })
})

describe('childProgressFromTree', () => {
  const child = (over: Partial<FlowRun>): FlowRun =>
    ({ ...run('R2', 'R1', 'R1'), parentNodeId: 'sub1', ...over }) as FlowRun

  it('seeds a rollup for a run that finished before the viewer opened', () => {
    // Without this the badge and the descend-into-child gesture only ever work
    // while watching a run live — but the runs list is normally opened after the
    // fact, so that is the common case, not the edge case.
    const state = JSON.stringify({
      trace: [
        { nodeId: 'a', type: 'delay', title: 'Bekle', output: '' },
        { nodeId: 'b', type: 'transform', title: 'Cikti', output: 'X' },
      ],
    })
    const map = childProgressFromTree([run('R1'), child({ state, status: 'success' })])
    expect(map.R1.sub1).toEqual<ChildProgress>({
      phase: 'done',
      title: 'Cikti',
      index: 2,
      runId: 'R2',
    })
  })

  it('maps every run status onto a lifecycle phase', () => {
    const of = (status: FlowRun['status']) =>
      childProgressFromTree([child({ status })]).R1.sub1.phase
    expect(of('running')).toBe('start')
    expect(of('waiting')).toBe('waiting')
    expect(of('success')).toBe('done')
    expect(of('failure')).toBe('error')
  })

  it('still records a child with no trace yet', () => {
    // The entry's real job is to say WHICH run hangs off the node — that is what
    // descending needs, with or without a title to show.
    const map = childProgressFromTree([child({ state: '', status: 'running' })])
    expect(map.R1.sub1).toMatchObject({ runId: 'R2', title: '', index: 0 })
  })

  it('survives a corrupt state instead of losing the whole tree', () => {
    const map = childProgressFromTree([child({ state: '{not json' })])
    expect(map.R1.sub1.runId).toBe('R2')
  })

  it('skips runs that hang off no node', () => {
    // The root itself, and run_flow children started outside any node.
    const map = childProgressFromTree([run('R1'), child({ id: 'R7', parentNodeId: undefined })])
    expect(map).toEqual({})
  })
})

describe('mergeChildProgress', () => {
  const seed = {
    R1: { sub1: { phase: 'done', title: 'eski', index: 1, runId: 'R2' } as ChildProgress },
  }

  it('lets a live frame win over the persisted seed', () => {
    const live = {
      R1: { sub1: { phase: 'start', title: 'yeni', index: 5, runId: 'R2' } as ChildProgress },
    }
    expect(mergeChildProgress(seed, live).R1.sub1).toMatchObject({ title: 'yeni', index: 5 })
  })

  it('keeps seeded nodes the live map says nothing about', () => {
    const live = {
      R1: { sub2: { phase: 'start', title: 'b', index: 1, runId: 'R3' } as ChildProgress },
    }
    const out = mergeChildProgress(seed, live)
    expect(out.R1.sub1.title).toBe('eski')
    expect(out.R1.sub2.runId).toBe('R3')
  })
})

describe('runTreeBreadcrumb', () => {
  const runs = [run('R1'), run('R2', 'R1', 'R1'), run('R3', 'R2', 'R1')]

  it('walks from the root down to the run', () => {
    expect(runTreeBreadcrumb(runs, 'R3').map((r) => r.id)).toEqual(['R1', 'R2', 'R3'])
  })

  it('is just the root itself at the top of the tree', () => {
    expect(runTreeBreadcrumb(runs, 'R1').map((r) => r.id)).toEqual(['R1'])
  })

  it('returns nothing for a run outside the tree', () => {
    expect(runTreeBreadcrumb(runs, 'NOPE')).toEqual([])
  })

  it('terminates on a corrupt parent cycle', () => {
    // A row pointing at its own descendant must not spin the render forever.
    const cyclic = [run('A', 'B'), run('B', 'A')]
    expect(runTreeBreadcrumb(cyclic, 'A')).toHaveLength(2)
  })
})
