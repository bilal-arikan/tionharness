import { describe, it, expect } from 'vitest'

import { flowRunRootOf, isRootFlowRun } from './flowRunTree'
import type { FlowRun } from '@/types'

// Only the lineage fields matter here; the rest is filler so the object is a
// valid FlowRun.
const run = (over: Partial<FlowRun>): FlowRun =>
  ({
    id: 'RUN1',
    flowId: 'F1',
    status: 'success',
    input: '',
    state: '',
    output: '',
    error: '',
    createdAt: 0,
    updatedAt: 0,
    ...over,
  }) as FlowRun

describe('flowRunRootOf', () => {
  it('reports a root run as its own root', () => {
    // The backend omits rootRunId on a root, so absent must NOT read as "unknown".
    expect(flowRunRootOf(run({ id: 'RUN1' }))).toBe('RUN1')
  })

  it('follows the recorded root of a descendant', () => {
    const child = run({ id: 'RUN2', parentRunId: 'RUN1', rootRunId: 'RUN1' })
    const grandchild = run({ id: 'RUN3', parentRunId: 'RUN2', rootRunId: 'RUN1' })
    // A grandchild points at the TOP of the tree, not at its parent — that is
    // what makes the tree fetch and the tree subscription single-keyed.
    expect(flowRunRootOf(child)).toBe('RUN1')
    expect(flowRunRootOf(grandchild)).toBe('RUN1')
  })

  it('treats an empty rootRunId as self even when the field is present', () => {
    // JSON round-trips can materialise the key as "" rather than dropping it.
    expect(flowRunRootOf(run({ id: 'RUN9', rootRunId: '' }))).toBe('RUN9')
  })
})

describe('isRootFlowRun', () => {
  it('decides on the parent link, not the root link', () => {
    expect(isRootFlowRun(run({ id: 'RUN1' }))).toBe(true)
    // A child records its own id nowhere in rootRunId terms that would help here:
    // rootRunId is set, parentRunId is set, and it is the parent link that rules.
    expect(isRootFlowRun(run({ id: 'RUN2', parentRunId: 'RUN1', rootRunId: 'RUN1' }))).toBe(false)
  })

  it('does not mistake the root of its own tree for a child', () => {
    // Defensive: a root that (redundantly) carries rootRunId === its own id is
    // still a root, because nothing launched it.
    expect(isRootFlowRun(run({ id: 'RUN1', rootRunId: 'RUN1' }))).toBe(true)
  })
})
