import type {
  AutomationTriggerKind,
  BoardAction,
  BoardOp,
  CounterMetric,
  CounterScope,
  SessionMode,
  TokenScope,
  TrajEndStatus,
  TrajEvent,
} from '@/types'
import { DEFAULT_MAX_ITERATIONS } from './automationMeta'
import { localInputToUnix } from './timeUtils'

export interface AutomationPayloadInput {
  kind: AutomationTriggerKind
  name: string
  triggerTag: string
  boardOp: BoardOp
  boardFromState: string
  boardToState: string
  boardPriority: number
  boardExclusive: boolean
  boardAction: BoardAction
  boardMoveToState: string
  tokenScope: TokenScope
  tokenThreshold: number
  counterMetric: CounterMetric
  counterScope: CounterScope
  counterInterval: number
  trajPhase: string
  trajRecipe: string
  trajEvent: TrajEvent
  trajStatus: TrajEndStatus
  targetMode: 'agent' | 'flow'
  targetAgentId: string
  flowId: string
  sessionMode: SessionMode
  promptTemplate: string
  maxIterations: string
  cooldownSec: string
  expiresAt: string
}

export function buildAutomationPayload(input: AutomationPayloadInput) {
  const isBoardKind = input.kind === 'board'
  const isTargetlessAction =
    isBoardKind && (input.boardAction === 'archive' || input.boardAction === 'move')
  const trigger = isBoardKind
    ? {
        boardOp: input.boardOp,
        boardFromState: input.boardFromState,
        boardToState: input.boardToState,
        boardPriority: input.boardPriority,
        boardExclusive: input.boardExclusive,
        boardAction: input.boardAction,
        boardMoveToState: input.boardMoveToState,
      }
    : input.kind === 'token'
      ? { tokenScope: input.tokenScope, tokenThreshold: input.tokenThreshold }
      : input.kind === 'counter'
        ? {
            counterMetric: input.counterMetric,
            counterScope: input.counterScope,
            counterInterval: input.counterInterval,
          }
        : input.kind === 'phase'
          ? {
              trajPhase: input.trajPhase.trim(),
              trajRecipe: input.trajRecipe.trim(),
              trajEvent: input.trajEvent,
            }
          : input.kind === 'trajectory_end'
            ? { trajRecipe: input.trajRecipe.trim(), trajStatus: input.trajStatus }
            : { triggerTag: input.triggerTag.trim() }
  const target = isTargetlessAction
    ? { targetAgentId: '', flowId: '' }
    : input.targetMode === 'flow'
      ? { flowId: input.flowId, targetAgentId: '' }
      : { targetAgentId: input.targetAgentId, flowId: '' }

  return {
    name: input.name.trim(),
    triggerKind: input.kind,
    ...trigger,
    ...target,
    ...(input.targetMode === 'agent' && !isTargetlessAction
      ? { sessionMode: input.sessionMode }
      : {}),
    promptTemplate: input.promptTemplate.trim(),
    maxIterations: Number(input.maxIterations) || DEFAULT_MAX_ITERATIONS,
    cooldownSec: Number(input.cooldownSec) || 0,
    expiresAt: localInputToUnix(input.expiresAt),
  }
}
