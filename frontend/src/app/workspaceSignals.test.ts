import { describe, expect, it } from 'vitest'
import { emptyLanes, type LaneState } from '@/shared/lib/laneModel'
import { diffSignals } from './workspaceSignals'

function withFires(s: LaneState, fires: LaneState['fires']): LaneState {
  return { ...s, fires }
}

describe('diffSignals', () => {
  it('toasts new fires only, skipping cooldown noise', () => {
    const a = emptyLanes()
    const b = withFires(a, [
      {
        seq: 1,
        at: 1,
        automationId: 'AUT1',
        name: 'Docs',
        triggerKind: 'tag',
        outcome: 'fired',
        sessionId: 'SES5',
      },
      {
        seq: 2,
        at: 2,
        automationId: 'AUT2',
        triggerKind: 'tag',
        outcome: 'skipped',
        reason: 'cooldown',
      },
      {
        seq: 3,
        at: 3,
        automationId: 'AUT3',
        triggerKind: 'tag',
        outcome: 'skipped',
        reason: 'max_iterations',
      },
    ])
    const out = diffSignals(a, b)
    expect(out.map((o) => o.text)).toEqual([
      '⚡ Otomasyon Docs ateşlendi → SES5',
      '↷ Otomasyon AUT3 atlandı: iterasyon tavanı',
    ])
    // Nothing new → nothing said.
    expect(diffSignals(b, b)).toEqual([])
  })

  it('announces trajectory start and terminal transitions, honouring the mute', () => {
    const a = emptyLanes()
    const b: LaneState = {
      ...a,
      trajectories: new Map([
        [
          'RTA1',
          {
            trajectoryId: 'RTA1',
            rootSessionId: 'SES1',
            op: 'create',
            templateRef: 'plan@3',
            status: 'planned',
            revision: 1,
            nodeCount: 4,
          },
        ],
      ]),
    }
    expect(diffSignals(a, b).map((o) => o.text)).toEqual(['◈ Rota RTA1 başladı · plan@3'])
    const c: LaneState = {
      ...b,
      trajectories: new Map([
        [
          'RTA1',
          {
            trajectoryId: 'RTA1',
            rootSessionId: 'SES1',
            op: 'update',
            status: 'done',
            revision: 9,
            nodeCount: 8,
          },
        ],
      ]),
    }
    expect(diffSignals(b, c)).toEqual([{ level: 'success', text: '◈ Rota RTA1 tamamlandı' }])
    expect(diffSignals(b, c, (t) => t !== 'rota')).toEqual([])
  })

  it('reports stall halts and flow failures', () => {
    const a = emptyLanes()
    const b: LaneState = {
      ...a,
      activity: [
        {
          seq: 1,
          kind: 'coordination',
          at: 1,
          coordinatorId: 'SES1',
          phase: 'stall_halt',
          reason: 'no spawn in 3 turns',
        },
      ],
      flowRuns: new Map([
        [
          'RUN1',
          {
            runId: 'RUN1',
            flowId: 'FLW1',
            rootRunId: 'RUN1',
            status: 'running',
            createdAt: 1,
            updatedAt: 1,
          },
        ],
      ]),
    }
    expect(diffSignals(a, b).map((o) => o.level)).toEqual(['error'])
    const c: LaneState = {
      ...b,
      flowRuns: new Map([
        [
          'RUN1',
          {
            runId: 'RUN1',
            flowId: 'FLW1',
            rootRunId: 'RUN1',
            status: 'failure',
            error: 'boom',
            createdAt: 1,
            updatedAt: 2,
          },
        ],
      ]),
    }
    expect(diffSignals(b, c)).toEqual([
      { level: 'error', text: 'Akış koşusu RUN1 başarısız: boom' },
    ])
  })
})
