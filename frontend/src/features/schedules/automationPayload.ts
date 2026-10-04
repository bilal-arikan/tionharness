import type {
  AutomationTriggerKind,
  BoardAction,
  BoardOp,
  FlowRuleStatus,
  SessionMode,
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
  trajPhase: string
  trajRecipe: string
  trajEvent: TrajEvent
  trajStatus: TrajEndStatus
  flowAgentId: string
  flowStatus: FlowRuleStatus
  flowMaxGrade: number
  targetAgentId: string
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
    : input.kind === 'phase'
      ? {
          trajPhase: input.trajPhase.trim(),
          trajRecipe: input.trajRecipe.trim(),
          trajEvent: input.trajEvent,
        }
      : input.kind === 'trajectory_end'
        ? { trajRecipe: input.trajRecipe.trim(), trajStatus: input.trajStatus }
        : input.kind === 'flow'
          ? {
              flowAgentId: input.flowAgentId.trim(),
              flowStatus: input.flowStatus,
              flowMaxGrade: input.flowMaxGrade,
            }
          : { triggerTag: input.triggerTag.trim() }
  const target = isTargetlessAction ? { targetAgentId: '' } : { targetAgentId: input.targetAgentId }

  return {
    name: input.name.trim(),
    triggerKind: input.kind,
    ...trigger,
    ...target,
    ...(!isTargetlessAction ? { sessionMode: input.sessionMode } : {}),
    promptTemplate: input.promptTemplate.trim(),
    maxIterations: Number(input.maxIterations) || DEFAULT_MAX_ITERATIONS,
    cooldownSec: Number(input.cooldownSec) || 0,
    expiresAt: localInputToUnix(input.expiresAt),
  }
}
