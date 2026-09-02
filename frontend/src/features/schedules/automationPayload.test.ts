import { describe, expect, it } from 'vitest'
import { buildAutomationPayload, type AutomationPayloadInput } from './automationPayload'

const baseInput: AutomationPayloadInput = {
  kind: 'board',
  name: ' Move reviewed cards ',
  triggerTag: '',
  boardOp: 'move',
  boardFromState: 'in_progress',
  boardToState: 'review',
  boardPriority: 2,
  boardExclusive: true,
  boardAction: 'move',
  boardMoveToState: 'done',
  tokenScope: 'session',
  tokenThreshold: 1_000,
  counterMetric: 'message',
  counterScope: 'session',
  counterInterval: 5,
  trajPhase: '',
  trajRecipe: '',
  trajEvent: 'exit',
  trajStatus: '',
  targetMode: 'agent',
  targetAgentId: 'stale-agent',
  flowId: 'stale-flow',
  sessionMode: 'continue',
  promptTemplate: '   ',
  maxIterations: '4',
  cooldownSec: '12',
  expiresAt: '',
}

describe('buildAutomationPayload', () => {
  it('builds a targetless board move contract with its explicit destination', () => {
    const payload = buildAutomationPayload(baseInput)

    expect(payload).toMatchObject({
      name: 'Move reviewed cards',
      triggerKind: 'board',
      boardOp: 'move',
      boardFromState: 'in_progress',
      boardToState: 'review',
      boardPriority: 2,
      boardExclusive: true,
      boardAction: 'move',
      boardMoveToState: 'done',
      targetAgentId: '',
      flowId: '',
      promptTemplate: '',
      maxIterations: 4,
      cooldownSec: 12,
      expiresAt: 0,
    })
    expect(payload).not.toHaveProperty('sessionMode')
  })
})
