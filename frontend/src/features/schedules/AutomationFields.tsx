import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Info } from 'lucide-react'
import type {
  Agent,
  TrajEndStatus,
  TrajEvent,
  AutomationTriggerKind,
  BoardAction,
  BoardColumnDef,
  BoardOp,
  FlowRuleStatus,
} from '@/types'
import {
  BOARD_ACTIONS,
  BOARD_OPS,
  BOARD_PROMPT_VARS,
  FLOW_PROMPT_VARS,
  FLOW_STATUSES,
  TRAJ_END_STATUSES,
  TRAJ_EVENTS,
  TRAJ_PROMPT_VARS,
  PROMPT_VARS,
} from './automationMeta'
import { inputCls } from './pickers'

const selCls =
  'rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]'

// BoardTriggerFields renders the op + source/target column filters for a
// board-triggered automation, plus the arbitration controls (fire order and
// exclusivity) that decide what happens when several rules watch the same
// column. Source is shown for move/any/delete, target for everything except
// delete.
export function BoardTriggerFields({
  op,
  from,
  to,
  priority,
  exclusive,
  action,
  columns,
  onChange,
}: {
  op: BoardOp
  from: string
  to: string
  priority: number
  exclusive: boolean
  action: BoardAction
  columns: BoardColumnDef[]
  onChange: (patch: {
    op?: BoardOp
    from?: string
    to?: string
    priority?: number
    exclusive?: boolean
    action?: BoardAction
  }) => void
}) {
  const { t } = useTranslation('schedules')
  const showFrom = op === 'move' || op === 'any' || op === 'delete'
  const showTo = op !== 'delete'
  return (
    <div className="flex flex-wrap items-center gap-2">
      <label
        className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]"
        title={t('fields.board.actionHint')}
      >
        {t('fields.board.action')}
        <select
          value={action}
          onChange={(e) => onChange({ action: e.target.value as BoardAction })}
          className={selCls}
        >
          {BOARD_ACTIONS.map((a) => (
            <option key={a.value} value={a.value}>
              {a.label}
            </option>
          ))}
        </select>
      </label>
      <label className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]">
        {t('fields.event')}
        <select
          value={op}
          onChange={(e) => onChange({ op: e.target.value as BoardOp })}
          className={selCls}
        >
          {BOARD_OPS.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
      </label>
      {showFrom && (
        <label className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]">
          {t('fields.source')}
          <select
            value={from}
            onChange={(e) => onChange({ from: e.target.value })}
            className={selCls}
          >
            <option value="">{t('common.any')}</option>
            {columns.map((c) => (
              <option key={c.key} value={c.key}>
                {c.label}
              </option>
            ))}
          </select>
        </label>
      )}
      {showTo && (
        <label className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]">
          {t('fields.target')}
          <select value={to} onChange={(e) => onChange({ to: e.target.value })} className={selCls}>
            <option value="">{t('common.any')}</option>
            {columns.map((c) => (
              <option key={c.key} value={c.key}>
                {c.label}
              </option>
            ))}
          </select>
        </label>
      )}
      <label
        className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]"
        title={t('fields.board.priorityHint')}
      >
        {t('fields.board.priority')}
        <input
          type="number"
          value={priority}
          onChange={(e) => onChange({ priority: Number(e.target.value) || 0 })}
          className={`${selCls} w-16`}
        />
      </label>
      <label
        className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]"
        title={t('fields.board.exclusiveHint')}
      >
        <input
          type="checkbox"
          checked={exclusive}
          onChange={(e) => onChange({ exclusive: e.target.checked })}
          className="accent-[var(--color-accent)]"
        />
        {t('fields.board.exclusive')}
      </label>
    </div>
  )
}

// PromptVarsField renders the prompt-template textarea plus the ℹ️ variable
// picker popover, choosing the variable list by trigger kind.
export function PromptVarsField({
  kind,
  value,
  onChange,
}: {
  kind: AutomationTriggerKind
  value: string
  onChange: (next: string) => void
}) {
  const { t } = useTranslation('schedules')
  const [show, setShow] = useState(false)
  const vars =
    kind === 'board'
      ? BOARD_PROMPT_VARS
      : kind === 'phase' || kind === 'trajectory_end'
        ? TRAJ_PROMPT_VARS
        : kind === 'flow'
          ? FLOW_PROMPT_VARS
          : PROMPT_VARS
  return (
    <div className="relative">
      <div className="mb-1 flex items-center gap-1 text-[11px] font-medium uppercase tracking-wide text-[var(--color-text-dim)]">
        <span>{t('fields.promptTemplate')}</span>
        <button
          type="button"
          onClick={() => setShow((v) => !v)}
          className={`rounded p-0.5 transition hover:text-[var(--color-accent)] ${show ? 'text-[var(--color-accent)]' : ''}`}
          title={t('fields.availableVariables')}
          aria-label={t('fields.availableVariables')}
        >
          <Info size={13} />
        </button>
      </div>
      {show && (
        <>
          <div className="fixed inset-0 z-10" onClick={() => setShow(false)} />
          <div className="absolute bottom-full left-0 z-20 mb-1 w-[360px] max-w-[90vw] rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] p-2 shadow-[var(--shadow-lg)]">
            <div className="mb-1 px-1 text-[11px] font-semibold text-[var(--color-text-dim)]">
              {t('fields.variablePickerHint')}
            </div>
            <div className="max-h-64 overflow-y-auto">
              {vars.map((v) => (
                <button
                  key={v.name}
                  type="button"
                  onClick={() => {
                    onChange(value + v.name)
                    setShow(false)
                  }}
                  className="flex w-full items-baseline gap-2 rounded px-1.5 py-1 text-left transition hover:bg-[var(--color-surface-2)]"
                  title={t('fields.addToTemplate')}
                >
                  <code className="shrink-0 rounded bg-[var(--color-accent-soft)] px-1 py-0.5 font-mono text-[11px] text-[var(--color-accent)]">
                    {v.name}
                  </code>
                  <span className="text-[11px] text-[var(--color-text-dim)]">{v.desc}</span>
                </button>
              ))}
            </div>
          </div>
        </>
      )}
      <textarea
        value={value}
        onChange={(e) => onChange(e.target.value)}
        rows={5}
        placeholder={t('fields.promptPlaceholder')}
        className={`${inputCls} resize-y`}
      />
    </div>
  )
}

// TrajectoryTriggerFields renders the filters of a Rota trigger (F2): a phase
// rule may narrow to one phase id and pick the transition (exit / enter); a
// trajectory_end rule may narrow to one terminal status; both may narrow to
// the recipe the trajectory was seeded from. Empty = any. Recipe watchers
// declared in a coordinator recipe (`watchers:`) fire the rule regardless of
// these filters — the recipe is the binding then.
export function TrajectoryTriggerFields({
  kind,
  phase,
  recipe,
  event,
  status,
  onChange,
}: {
  kind: 'phase' | 'trajectory_end'
  phase: string
  recipe: string
  event: TrajEvent
  status: TrajEndStatus
  onChange: (patch: {
    phase?: string
    recipe?: string
    event?: TrajEvent
    status?: TrajEndStatus
  }) => void
}) {
  const { t } = useTranslation('schedules')
  return (
    <div className="flex flex-wrap items-center gap-2">
      {kind === 'phase' ? (
        <>
          <label
            className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]"
            title={t('fields.trajectory.phaseHint')}
          >
            {t('fields.phase')}
            <input
              value={phase}
              onChange={(e) => onChange({ phase: e.target.value })}
              placeholder={t('fields.trajectory.anyPhase')}
              className={`${inputCls} w-28 font-mono`}
            />
          </label>
          <label className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]">
            {t('fields.event')}
            <select
              value={event}
              onChange={(e) => onChange({ event: e.target.value as TrajEvent })}
              className={selCls}
            >
              {TRAJ_EVENTS.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </select>
          </label>
        </>
      ) : (
        <label className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]">
          {t('fields.endStatus')}
          <select
            value={status}
            onChange={(e) => onChange({ status: e.target.value as TrajEndStatus })}
            className={selCls}
          >
            {TRAJ_END_STATUSES.map((o) => (
              <option key={o.value || 'any'} value={o.value}>
                {o.label}
              </option>
            ))}
          </select>
        </label>
      )}
      <label
        className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]"
        title={t('fields.trajectory.recipeHint')}
      >
        {t('fields.recipe')}
        <input
          value={recipe}
          onChange={(e) => onChange({ recipe: e.target.value })}
          placeholder={t('fields.trajectory.anyRecipe')}
          className={`${inputCls} w-36 font-mono`}
        />
      </label>
    </div>
  )
}

// FlowTriggerFields renders the filters of a flow-run trigger (_Docs/93): the
// agent whose flow finished (empty = any), the run's outcome, and an optional
// grade ceiling — "fire only when the decision model graded the reply at or
// below N" (0 = ignore the grade; needs the flow-grade authority on).
export function FlowTriggerFields({
  agents,
  agentId,
  status,
  maxGrade,
  onChange,
}: {
  agents: Agent[]
  agentId: string
  status: FlowRuleStatus
  maxGrade: number
  onChange: (patch: { agentId?: string; status?: FlowRuleStatus; maxGrade?: number }) => void
}) {
  const { t } = useTranslation('schedules')
  return (
    <div className="flex flex-wrap items-center gap-2">
      <label
        className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]"
        title={t('fields.flow.agentHint')}
      >
        {t('fields.flow.agent')}
        <select
          value={agentId}
          onChange={(e) => onChange({ agentId: e.target.value })}
          className={selCls}
          data-testid="automation-flow-agent"
        >
          <option value="">{t('fields.flow.anyAgent')}</option>
          {agents
            .filter((a) => !a.system && !a.deleted)
            .map((a) => (
              <option key={a.id} value={a.id}>
                {a.name}
              </option>
            ))}
        </select>
      </label>
      <label className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]">
        {t('fields.flow.status')}
        <select
          value={status}
          onChange={(e) => onChange({ status: e.target.value as FlowRuleStatus })}
          className={selCls}
        >
          {FLOW_STATUSES.map((o) => (
            <option key={o.value || 'any'} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
      </label>
      <label
        className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]"
        title={t('fields.flow.maxGradeHint')}
      >
        {t('fields.flow.maxGrade')}
        <select
          value={maxGrade}
          onChange={(e) => onChange({ maxGrade: Number(e.target.value) || 0 })}
          className={selCls}
        >
          <option value={0}>{t('fields.flow.ignoreGrade')}</option>
          {[1, 2, 3, 4, 5].map((n) => (
            <option key={n} value={n}>
              {t('fields.flow.gradeAtMost', { n })}
            </option>
          ))}
        </select>
      </label>
    </div>
  )
}
