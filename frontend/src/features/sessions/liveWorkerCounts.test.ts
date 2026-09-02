import { describe, expect, it } from 'vitest'
import { countLiveDescendantWorkers, type RuntimeLineage } from './liveWorkerCounts'

const runtime = (entries: Array<[string, RuntimeLineage]>) => new Map(entries)

describe('countLiveDescendantWorkers', () => {
  it('counts two live deep leaves at both ancestors and ignores a completed leaf', () => {
    const runtimeById = runtime([
      ['SES2937', { running: false, rootCoordinatorSessionId: 'SES2937' }],
      [
        'SES2980',
        {
          running: false,
          coordinatorSessionId: 'SES2937',
          rootCoordinatorSessionId: 'SES2937',
        },
      ],
      [
        'SES3015',
        {
          running: true,
          coordinatorSessionId: 'SES2980',
          rootCoordinatorSessionId: 'SES2937',
        },
      ],
      [
        'SES3016',
        {
          running: true,
          coordinatorSessionId: 'SES2980',
          rootCoordinatorSessionId: 'SES2937',
        },
      ],
      [
        'SES3017',
        {
          running: false,
          coordinatorSessionId: 'SES2980',
          rootCoordinatorSessionId: 'SES2937',
        },
      ],
    ])

    const counts = countLiveDescendantWorkers([], runtimeById)

    expect(counts.get('SES2937')).toBe(2)
    expect(counts.get('SES2980')).toBe(2)
  })

  it('keeps a sibling root isolated', () => {
    const runtimeById = runtime([
      [
        'LEAF_A',
        { running: true, coordinatorSessionId: 'ROOT_A', rootCoordinatorSessionId: 'ROOT_A' },
      ],
      [
        'LEAF_B',
        { running: true, coordinatorSessionId: 'ROOT_B', rootCoordinatorSessionId: 'ROOT_B' },
      ],
    ])

    const counts = countLiveDescendantWorkers([], runtimeById)

    expect(counts.get('ROOT_A')).toBe(1)
    expect(counts.get('ROOT_B')).toBe(1)
  })

  it('counts a paged-out worker found only in the runtime map', () => {
    const sessions = [
      { id: 'ROOT', rootCoordinatorSessionId: 'ROOT' },
      { id: 'MID', coordinatorSessionId: 'ROOT', rootCoordinatorSessionId: 'ROOT' },
    ]
    const runtimeById = runtime([
      ['LEAF', { running: true, coordinatorSessionId: 'MID', rootCoordinatorSessionId: 'ROOT' }],
    ])

    const counts = countLiveDescendantWorkers(sessions, runtimeById)

    expect(counts.get('ROOT')).toBe(1)
    expect(counts.get('MID')).toBe(1)
  })

  it('terminates cycles without counting the live worker itself', () => {
    const runtimeById = runtime([
      ['A', { running: true, coordinatorSessionId: 'B', rootCoordinatorSessionId: 'A' }],
      ['B', { running: false, coordinatorSessionId: 'A', rootCoordinatorSessionId: 'A' }],
    ])

    const counts = countLiveDescendantWorkers([], runtimeById)

    expect(counts.get('B')).toBe(1)
    expect(counts.has('A')).toBe(false)
  })

  it('falls back to the root when an intermediate parent is missing', () => {
    const runtimeById = runtime([
      [
        'LEAF',
        { running: true, coordinatorSessionId: 'MISSING', rootCoordinatorSessionId: 'ROOT' },
      ],
    ])

    const counts = countLiveDescendantWorkers(
      [{ id: 'ROOT', rootCoordinatorSessionId: 'ROOT' }],
      runtimeById,
    )

    expect(counts.get('ROOT')).toBe(1)
  })

  it('uses loaded session lineage for a local stream before runtime refresh', () => {
    const sessions = [
      { id: 'ROOT', rootCoordinatorSessionId: 'ROOT' },
      { id: 'LEAF', coordinatorSessionId: 'ROOT', rootCoordinatorSessionId: 'ROOT' },
    ]

    const counts = countLiveDescendantWorkers(sessions, undefined, new Set(['LEAF']))

    expect(counts.get('ROOT')).toBe(1)
  })

  it('deduplicates server running state and local streaming state', () => {
    const runtimeById = runtime([
      ['LEAF', { running: true, coordinatorSessionId: 'ROOT', rootCoordinatorSessionId: 'ROOT' }],
    ])

    const counts = countLiveDescendantWorkers([], runtimeById, new Set(['LEAF']))

    expect(counts.get('ROOT')).toBe(1)
  })

  it('does not count a running root or normal chat as its own descendant', () => {
    const runtimeById = runtime([
      ['ROOT', { running: true, rootCoordinatorSessionId: 'ROOT' }],
      ['CHAT', { running: true }],
    ])

    expect(countLiveDescendantWorkers([], runtimeById)).toEqual(new Map())
  })

  it('clears the count when the worker finishes', () => {
    const running = runtime([
      ['LEAF', { running: true, coordinatorSessionId: 'ROOT', rootCoordinatorSessionId: 'ROOT' }],
    ])
    const finished = runtime([
      ['LEAF', { running: false, coordinatorSessionId: 'ROOT', rootCoordinatorSessionId: 'ROOT' }],
    ])

    expect(countLiveDescendantWorkers([], running).get('ROOT')).toBe(1)
    expect(countLiveDescendantWorkers([], finished)).toEqual(new Map())
  })
})
