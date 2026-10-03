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
  trajPhase: '',
  trajRecipe: '',
  trajEvent: 'exit',
  trajStatus: '',
  flowAgentId: '',
  flowStatus: '',
  flowMaxGrade: 0,
  targetAgentId: 'stale-agent',
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
      promptTemplate: '',
      maxIterations: 4,
      cooldownSec: 12,
      expiresAt: 0,
    })
    expect(payload).not.toHaveProperty('sessionMode')
  })

  it('builds a flow-run trigger with its agent, outcome and grade filters', () => {
    const body = buildAutomationPayload({
      ...baseInput,
      kind: 'flow',
      flowAgentId: ' AGT7 ',
      flowStatus: 'failure',
      flowMaxGrade: 2,
      promptTemplate: 'Review {{result}}',
    })
    expect(body).toMatchObject({
      triggerKind: 'flow',
      flowAgentId: 'AGT7',
      flowStatus: 'failure',
      flowMaxGrade: 2,
      targetAgentId: 'stale-agent',
      sessionMode: 'continue',
      promptTemplate: 'Review {{result}}',
    })
    expect(body).not.toHaveProperty('triggerTag')
    expect(body).not.toHaveProperty('tokenThreshold')
  })
})
