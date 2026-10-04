import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { LayoutGrid, Repeat, Sparkles, Workflow, X } from 'lucide-react'
import { api } from '@/api'
import type {
  Agent,
  Automation,
  AutomationTriggerKind,
  BoardAction,
  BoardColumnDef,
  BoardOp,
  FlowRuleStatus,
  SessionMode,
  TrajEndStatus,
  TrajEvent,
} from '@/types'
import { AgentPicker } from '@/shared/components/agents/AgentPicker'
import { toast } from '@/shared/components'
import {
  COLUMN_ACCENT,
  DEFAULT_MAX_ITERATIONS,
  DEFAULT_PROMPT,
  MAX_ITERATIONS_HARD_CAP,
  STUCK_TEMPLATE,
} from './automationMeta'
import {
  BoardTriggerFields,
  FlowTriggerFields,
  TrajectoryTriggerFields,
  PromptVarsField,
} from './AutomationFields'
import { FieldError } from './FieldError'
import { useFieldErrors } from './useFieldErrors'
import { FormModal } from './FormModal'
import { Field, inputCls } from './pickers'
import { localInputToUnix, unixToLocalInput } from './timeUtils'
import { buildAutomationPayload } from './automationPayload'

interface Props {
  kind: AutomationTriggerKind
  agents: Agent[]
  columns: BoardColumnDef[]
  /** null = create a new automation; otherwise edit this one. */
  editing: Automation | null
  onClose: () => void
  onSaved: (a: Automation | null, isNew: boolean) => void
  /** Edit mode only: delete this automation (the board confirms and closes). */
  onDelete?: () => void
  onError: (msg: string) => void
}

// AutomationModal is the create/edit popup for both automation kinds (tag and
// board columns). The trigger block swaps by kind; everything else is shared.
export function AutomationModal({
  kind,
  agents,
  columns,
  editing,
  onClose,
  onSaved,
  onDelete,
  onError,
}: Props) {
  const { t } = useTranslation('schedules')
  const isBoardKind = kind === 'board'
  const isTrajKind = kind === 'phase' || kind === 'trajectory_end'
  const isFlowKind = kind === 'flow'

  const [name, setName] = useState(editing?.name ?? '')
  const [triggerTag, setTriggerTag] = useState(editing?.triggerTag ?? '')
  const [boardOp, setBoardOp] = useState<BoardOp>(editing?.boardOp ?? 'move')
  const [boardFromState, setBoardFromState] = useState(editing?.boardFromState ?? '')
  const [boardToState, setBoardToState] = useState(editing?.boardToState ?? '')
  const [boardPriority, setBoardPriority] = useState(editing?.boardPriority ?? 0)
  const [boardExclusive, setBoardExclusive] = useState(editing?.boardExclusive ?? false)
  const [boardAction, setBoardAction] = useState<BoardAction>(editing?.boardAction ?? 'spawn')
  const [boardMoveToState, setBoardMoveToState] = useState(editing?.boardMoveToState ?? '')
  const [trajPhase, setTrajPhase] = useState(editing?.trajPhase ?? '')
  const [trajRecipe, setTrajRecipe] = useState(editing?.trajRecipe ?? '')
  const [trajEvent, setTrajEvent] = useState<TrajEvent>(editing?.trajEvent ?? 'exit')
  const [trajStatus, setTrajStatus] = useState<TrajEndStatus>(editing?.trajStatus ?? '')
  const [flowAgentId, setFlowAgentId] = useState(editing?.flowAgentId ?? '')
  const [flowStatus, setFlowStatus] = useState<FlowRuleStatus>(editing?.flowStatus ?? '')
  const [flowMaxGrade, setFlowMaxGrade] = useState(editing?.flowMaxGrade ?? 0)
  const [targetAgentId, setTargetAgentId] = useState(editing?.targetAgentId ?? '')
  // Session strategy: default matches the backend's default (spawn a fresh session).
  const [sessionMode, setSessionMode] = useState<SessionMode>(editing?.sessionMode ?? 'spawn')
  const [promptTemplate, setPromptTemplate] = useState(
    editing?.promptTemplate ?? DEFAULT_PROMPT[kind],
  )
  const [maxIterations, setMaxIterations] = useState(
    String(editing?.maxIterations ?? DEFAULT_MAX_ITERATIONS),
  )
  const [cooldownSec, setCooldownSec] = useState(String(editing?.cooldownSec ?? 0))
  const [expiresAt, setExpiresAt] = useState(unixToLocalInput(editing?.expiresAt))
  // spawnTagsOverride: set by a template (e.g. stuck repair must NOT re-tag the
  // fixer, or it would loop); null = backend default ([triggerTag]).
  const [spawnTagsOverride, setSpawnTagsOverride] = useState<string[] | null>(null)
  const [generatingTitle, setGeneratingTitle] = useState(false)

  const isTargetlessAction = isBoardKind && (boardAction === 'archive' || boardAction === 'move')
  const missingTarget = !isTargetlessAction && !targetAgentId
  // Required-field errors, mirroring db.ValidateAutomationShape. Record order is
  // the blocking priority; useFieldErrors gates each behind a submit attempt.
  const { markAttempted, firstError, errorFor } = useFieldErrors({
    tag: kind === 'tag' && !triggerTag.trim() ? t('validation.triggerTagRequired') : '',
    moveTarget:
      isBoardKind && boardAction === 'move' && !boardMoveToState
        ? t('validation.moveTargetRequired')
        : '',
    target: missingTarget ? t('validation.targetAgentRequired') : '',
  })

  const applyStuckTemplate = () => {
    setName(STUCK_TEMPLATE.name)
    setTriggerTag(STUCK_TEMPLATE.triggerTag)
    setPromptTemplate(STUCK_TEMPLATE.promptTemplate)
    setMaxIterations(STUCK_TEMPLATE.maxIterations)
    setCooldownSec(STUCK_TEMPLATE.cooldownSec)
    setSpawnTagsOverride([]) // the fixer itself must not carry `stuck`
  }

  const submit = async () => {
    markAttempted()
    // Prompt is required for every rule except an archive board rule (no LLM call).
    if (!isTargetlessAction && !promptTemplate.trim()) {
      onError(t('validation.promptTemplateRequired'))
      return
    }
    // Shape guards, mirroring db.ValidateAutomationShape. The inline messages under
    // each field carry the detail; the toast is the catch-all for the first blocker.
    if (firstError) {
      onError(firstError)
      return
    }
    const expUnix = localInputToUnix(expiresAt)
    if (expUnix && expUnix <= Math.floor(Date.now() / 1000)) {
      onError(t('validation.expiryFuture'))
      return
    }
    const body = buildAutomationPayload({
      kind,
      name,
      triggerTag,
      boardOp,
      boardFromState,
      boardToState,
      boardPriority,
      boardExclusive,
      boardAction,
      boardMoveToState,
      trajPhase,
      trajRecipe,
      trajEvent,
      trajStatus,
      flowAgentId,
      flowStatus,
      flowMaxGrade,
      targetAgentId,
      sessionMode,
      promptTemplate,
      maxIterations,
      cooldownSec,
      expiresAt,
    })
    try {
      if (editing) {
        await api.updateAutomation(editing.id, body)
        onSaved(null, false) // caller reloads the list
      } else {
        const created = await api.createAutomation({
          ...body,
          ...(spawnTagsOverride !== null ? { spawnTags: spawnTagsOverride } : {}),
          enabled: true,
        })
        onSaved(created, true)
      }
      toast.success(editing ? t('automationModal.updated') : t('automationModal.created'))
      onClose()
    } catch (e) {
      onError((e as Error).message)
    }
  }

  const kindLabel = t(`automationKinds.${kind}`)

  return (
    <FormModal
      title={t(editing ? 'automationModal.editTitle' : 'automationModal.newTitle', {
        kind: kindLabel,
      })}
      icon={isBoardKind ? LayoutGrid : isFlowKind ? Workflow : Repeat}
      accent={
        isBoardKind ? COLUMN_ACCENT.board : isFlowKind ? COLUMN_ACCENT.flow : COLUMN_ACCENT.tag
      }
      submitLabel={editing ? t('common.save') : t('automationModal.add')}
      onSubmit={submit}
      onClose={onClose}
      onDelete={editing ? onDelete : undefined}
      deleteTestId="automation-delete"
      testId={`automation-${kind}-modal`}
    >
      <Field label={t('common.nameOptional')}>
        <div className="flex items-center gap-2">
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={t('automationModal.namePlaceholder')}
            className={inputCls}
          />
          {editing && (
            <button
              type="button"
              disabled={generatingTitle}
              onClick={async () => {
                setGeneratingTitle(true)
                try {
                  // Suggestion only — it lands in the form and is persisted by Save,
                  // so Cancel still discards it.
                  const { title } = await api.generateAutomationTitle(editing.id)
                  setName(title)
                  toast.success(t('common.titleSuggested'))
                } catch (e) {
                  onError((e as Error).message)
                } finally {
                  setGeneratingTitle(false)
                }
              }}
              className="flex shrink-0 items-center gap-1 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-xs text-[var(--color-text-dim)] hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
              title={t('common.generateTitle')}
            >
              <Sparkles size={14} className={generatingTitle ? 'animate-pulse' : ''} />
              {generatingTitle ? '...' : t('common.generate')}
            </button>
          )}
        </div>
      </Field>

      <Field label={t('fields.trigger')}>
        {isBoardKind ? (
          <BoardTriggerFields
            op={boardOp}
            from={boardFromState}
            to={boardToState}
            priority={boardPriority}
            exclusive={boardExclusive}
            action={boardAction}
            columns={columns}
            onChange={(p) => {
              if (p.op !== undefined) setBoardOp(p.op)
              if (p.from !== undefined) setBoardFromState(p.from)
              if (p.to !== undefined) setBoardToState(p.to)
              if (p.priority !== undefined) setBoardPriority(p.priority)
              if (p.exclusive !== undefined) setBoardExclusive(p.exclusive)
              if (p.action !== undefined) setBoardAction(p.action)
            }}
          />
        ) : isFlowKind ? (
          <FlowTriggerFields
            agents={agents}
            agentId={flowAgentId}
            status={flowStatus}
            maxGrade={flowMaxGrade}
            onChange={(p) => {
              if (p.agentId !== undefined) setFlowAgentId(p.agentId)
              if (p.status !== undefined) setFlowStatus(p.status)
              if (p.maxGrade !== undefined) setFlowMaxGrade(p.maxGrade)
            }}
          />
        ) : isTrajKind ? (
          <TrajectoryTriggerFields
            kind={kind}
            phase={trajPhase}
            recipe={trajRecipe}
            event={trajEvent}
            status={trajStatus}
            onChange={(p) => {
              if (p.phase !== undefined) setTrajPhase(p.phase)
              if (p.recipe !== undefined) setTrajRecipe(p.recipe)
              if (p.event !== undefined) setTrajEvent(p.event)
              if (p.status !== undefined) setTrajStatus(p.status)
            }}
          />
        ) : (
          <input
            value={triggerTag}
            onChange={(e) => setTriggerTag(e.target.value)}
            placeholder={t('automationModal.triggerTagPlaceholder')}
            className={`${inputCls} font-mono ${errorFor('tag') ? 'border-[var(--color-danger)]' : ''}`}
          />
        )}
        <FieldError message={errorFor('tag')} />
      </Field>

      {isTargetlessAction ? (
        <div className="rounded border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs text-[var(--color-text-dim)]">
          {boardAction === 'archive' ? (
            <>{t('automationModal.archiveActionDescription')}</>
          ) : (
            <label className="flex items-center gap-2">
              {t('automationModal.targetColumn')}
              <select
                value={boardMoveToState}
                onChange={(e) => setBoardMoveToState(e.target.value)}
                className={inputCls}
              >
                <option value="">{t('common.select')}</option>
                {columns.map((column) => (
                  <option key={column.key} value={column.key}>
                    {column.label}
                  </option>
                ))}
              </select>
            </label>
          )}
          <FieldError message={errorFor('moveTarget')} />
        </div>
      ) : (
        <>
          <Field label={t('fields.target')} hint={t('automationModal.targetHint')}>
            <div className="flex flex-wrap items-center gap-2">
              <AgentPicker agents={agents} value={targetAgentId} onChange={setTargetAgentId} />
            </div>
            <FieldError message={errorFor('target')} />
          </Field>

          <Field label={t('fields.session')} hint={t('automationModal.sessionHint')}>
            <div className="inline-flex overflow-hidden rounded border border-[var(--color-border)] text-xs">
              {(
                [
                  ['spawn', t('automationModal.sessionSpawn')],
                  ['continue', t('automationModal.sessionContinue')],
                ] as [SessionMode, string][]
              ).map(([m, label]) => (
                <button
                  key={m}
                  type="button"
                  onClick={() => setSessionMode(m)}
                  className={`px-2.5 py-1 transition ${
                    sessionMode === m
                      ? 'bg-[var(--color-accent)] text-[var(--color-on-accent)]'
                      : 'bg-[var(--color-bg)] text-[var(--color-text-dim)] hover:text-[var(--color-accent)]'
                  }`}
                >
                  {label}
                </button>
              ))}
            </div>
            {sessionMode === 'continue' && (
              <p className="mt-1 text-[11px] text-[var(--color-text-dim)] opacity-80">
                {t('automationModal.continueDescription')}
              </p>
            )}
          </Field>

          <PromptVarsField kind={kind} value={promptTemplate} onChange={setPromptTemplate} />
        </>
      )}

      <div className="flex flex-wrap items-end gap-3">
        <label className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]">
          {t('automationModal.maxIterations')}
          <input
            type="number"
            min={1}
            max={MAX_ITERATIONS_HARD_CAP}
            value={maxIterations}
            onChange={(e) => setMaxIterations(e.target.value)}
            className="w-20 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none"
            title={t('automationModal.maxIterationsHint', { max: MAX_ITERATIONS_HARD_CAP })}
          />
        </label>
        <label className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]">
          {t('automationModal.cooldownSeconds')}
          <input
            type="number"
            min={0}
            value={cooldownSec}
            onChange={(e) => setCooldownSec(e.target.value)}
            className="w-20 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none"
            title={t('automationModal.cooldownHint')}
          />
        </label>
        <label className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]">
          {t('common.expiresOptionalShort')}
          <input
            type="datetime-local"
            value={expiresAt}
            onChange={(e) => setExpiresAt(e.target.value)}
            className="rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]"
            title={t('automationModal.expiryHint')}
          />
          {expiresAt && (
            <button
              type="button"
              onClick={() => setExpiresAt('')}
              className="text-[var(--color-text-dim)] hover:text-[var(--color-danger)]"
              title={t('common.clearExpiry')}
            >
              <X size={13} />
            </button>
          )}
        </label>
      </div>

      {kind === 'tag' && !editing && (
        <div className="flex flex-wrap items-center gap-2 border-t border-[var(--color-border)] pt-3 text-xs text-[var(--color-text-dim)]">
          {t('automationModal.template')}:
          <button
            type="button"
            onClick={applyStuckTemplate}
            className="rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-0.5 font-medium text-[var(--color-text)] hover:border-[var(--color-accent)]"
            title={t('automationModal.stuckTemplateHint')}
          >
            🩹 {t('automationModal.stuckTemplate')}
          </button>
          {spawnTagsOverride !== null && (
            <span className="text-[var(--color-warning)]">
              {t('automationModal.stuckTemplateActive')}
            </span>
          )}
        </div>
      )}
    </FormModal>
  )
}
