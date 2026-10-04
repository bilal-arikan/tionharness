import {
  RotateCcw,
  Pencil,
  LayoutGrid,
  Archive,
  MoveRight,
  Waypoints,
  Flag,
  Workflow,
  Pin,
  PinOff,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { Agent, Automation, BoardColumnDef } from '@/types'
import { AgentAvatar } from '@/shared/components/agents/AgentAvatar'
import { TagEditor } from '@/shared/components'
import { CardAction } from './pickers'
import { COLUMN_ACCENT, boardOpLabel } from './automationMeta'
import { fmtTime, isPast } from './timeUtils'
import { AutomationFires } from './AutomationFires'

interface Props {
  automation: Automation
  isBoardKind: boolean
  agents: Agent[]
  columns: BoardColumnDef[]
  onToggle: () => void
  onReset: () => void
  // Archive: hide + stop without deleting (restorable via the API/curator).
  onArchive: () => void
  // Pin / unpin (Rota F3): a pinned rule is exempt from the curator.
  onPin?: () => void
  onEdit: () => void
  onSpawnTags: (tags: string[]) => void
}

// AutomationCard is one event-driven rule rendered as a board card. It serves
// both kinds: the trigger chip on top is either `#tag` (tag column) or the board
// op + column filter (board column). Deleting is not offered here — it lives
// inside the edit popup.
export function AutomationCard({
  automation: a,
  isBoardKind,
  agents,
  columns,
  onToggle,
  onReset,
  onArchive,
  onPin,
  onEdit,
  onSpawnTags,
}: Props) {
  const { t } = useTranslation('schedules')
  const owner = agents.find((x) => x.id === a.targetAgentId)
  const colLabel = (key?: string) =>
    key ? (columns.find((c) => c.key === key)?.label ?? key) : '—'
  const maxed = a.maxIterations > 0 && a.iterationCount >= a.maxIterations
  const expired = isPast(a.expiresAt)
  const opLabel = boardOpLabel(a.boardOp)
  const isTargetlessRule = isBoardKind && (a.boardAction === 'archive' || a.boardAction === 'move')
  // Effective session mode (agent-backed only): explicit value wins, else spawn.
  const kind = a.triggerKind ?? 'tag'
  const effectiveMode = a.sessionMode ?? 'spawn'
  const showContinue = !isTargetlessRule && effectiveMode === 'continue'

  return (
    <div
      data-testid="automation-row"
      data-automation-id={a.id}
      className="rounded-lg border border-l-4 border-[var(--color-border)] bg-[var(--color-surface)] px-2.5 py-2 text-sm"
      style={{
        borderLeftColor: isBoardKind
          ? COLUMN_ACCENT.board
          : kind === 'phase'
            ? COLUMN_ACCENT.phase
            : kind === 'trajectory_end'
              ? COLUMN_ACCENT.trajectory_end
              : COLUMN_ACCENT.tag,
      }}
    >
      <div className="flex items-start gap-2">
        <div className="flex shrink-0 flex-col items-center gap-1.5">
          <button
            type="button"
            role="switch"
            aria-checked={a.enabled}
            aria-label={a.enabled ? t('common.enabled') : t('common.disabled')}
            onClick={onToggle}
            className={`h-4 w-8 rounded-full transition ${
              a.enabled ? 'bg-[var(--color-accent)]' : 'bg-[var(--color-border)]'
            }`}
            title={a.enabled ? t('common.enabled') : t('common.disabled')}
          >
            <span
              className={`block h-4 w-4 rounded-full bg-[var(--color-text)] transition ${a.enabled ? 'translate-x-4' : ''}`}
            />
          </button>
          {isTargetlessRule ? (
            <span
              className="flex h-7 w-7 items-center justify-center rounded-full bg-[var(--color-surface-2)] text-[var(--color-text-dim)]"
              title={t('automationCard.targetlessBoardAction')}
            >
              {a.boardAction === 'archive' ? <Archive size={15} /> : <MoveRight size={15} />}
            </span>
          ) : owner ? (
            <AgentAvatar agent={owner} size={28} />
          ) : (
            <span className="flex h-7 w-7 items-center justify-center rounded-full bg-[var(--color-surface-2)] text-[10px] text-[var(--color-text-dim)]">
              ?
            </span>
          )}
        </div>

        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-1">
            {isBoardKind ? (
              <span
                className="flex items-center gap-1 rounded bg-[var(--color-accent-soft)] px-1.5 py-0.5 text-[11px] text-[var(--color-accent)]"
                title={t('automationCard.boardTriggered')}
              >
                <LayoutGrid size={11} />
                {opLabel}
                {(a.boardFromState || a.boardToState) && (
                  <span className="opacity-80">
                    {t('automationCard.boardTransition', {
                      from: a.boardFromState ? colLabel(a.boardFromState) : t('common.wildcard'),
                      to: a.boardToState ? colLabel(a.boardToState) : t('common.wildcard'),
                    })}
                  </span>
                )}
                {a.boardAction === 'archive' && (
                  <span
                    className="flex items-center gap-0.5 opacity-80"
                    title={t('automationCard.archivesCard')}
                  >
                    <Archive size={10} /> {t('common.archive')}
                  </span>
                )}
                {a.boardAction === 'move' && (
                  <span
                    className="flex items-center gap-0.5 opacity-80"
                    title={t('automationCard.movesCard')}
                  >
                    <MoveRight size={10} /> {colLabel(a.boardMoveToState)}
                  </span>
                )}
              </span>
            ) : kind === 'phase' ? (
              <span
                className="flex items-center gap-1 rounded bg-[var(--color-accent-soft)] px-1.5 py-0.5 text-[11px] text-[var(--color-accent)]"
                title={t('automationCard.phaseTriggered')}
              >
                <Waypoints size={11} />
                {t('automationCard.phaseSummary', {
                  phase: a.trajPhase || t('common.wildcard'),
                  event:
                    a.trajEvent === 'enter'
                      ? t('automationCard.onEnter')
                      : t('automationCard.onExit'),
                })}
                {a.trajRecipe && (
                  <span className="opacity-80">
                    {t('common.dotValue', { value: a.trajRecipe })}
                  </span>
                )}
              </span>
            ) : kind === 'flow' ? (
              <span
                className="flex items-center gap-1 rounded bg-[var(--color-accent-soft)] px-1.5 py-0.5 text-[11px] text-[var(--color-accent)]"
                title={t('automationCard.flowTriggered')}
              >
                <Workflow size={11} />
                {t('automationCard.flowSummary', {
                  agent: a.flowAgentId
                    ? (agents.find((x) => x.id === a.flowAgentId)?.name ?? a.flowAgentId)
                    : t('common.wildcard'),
                  status: a.flowStatus
                    ? t(`meta.flowStatuses.${a.flowStatus}`)
                    : t('meta.flowStatuses.any'),
                })}
                {(a.flowMaxGrade ?? 0) > 0 && (
                  <span className="opacity-80">
                    {t('automationCard.flowGrade', { n: a.flowMaxGrade })}
                  </span>
                )}
              </span>
            ) : kind === 'trajectory_end' ? (
              <span
                className="flex items-center gap-1 rounded bg-[var(--color-accent-soft)] px-1.5 py-0.5 text-[11px] text-[var(--color-accent)]"
                title={t('automationCard.trajectoryEndTriggered')}
              >
                <Flag size={11} />
                {t('automationCard.trajectoryEndSummary', {
                  status: a.trajStatus
                    ? t(`meta.trajectoryStatuses.${a.trajStatus}`)
                    : t('meta.trajectoryStatuses.any'),
                })}
                {a.trajRecipe && (
                  <span className="opacity-80">
                    {t('common.dotValue', { value: a.trajRecipe })}
                  </span>
                )}
              </span>
            ) : (
              <span className="rounded bg-[var(--color-accent-soft)] px-1.5 py-0.5 font-mono text-[11px] text-[var(--color-accent)]">
                #{a.triggerTag}
              </span>
            )}
            {a.pinned && (
              <span
                className="rounded bg-[var(--color-bg)] px-1.5 py-0.5 text-[11px] text-[var(--color-text-dim)]"
                title={t('automationCard.pinnedTitle')}
              >
                📌 {t('automationCard.pinned')}
              </span>
            )}
            {isBoardKind && a.boardExclusive && (
              <span
                className="rounded bg-[color-mix(in_srgb,var(--color-warning)_16%,transparent)] px-1.5 py-0.5 text-[11px] text-[var(--color-text)]"
                title={t('automationCard.exclusiveTitle')}
              >
                🔒 {t('fields.board.exclusive')}
              </span>
            )}
            {isBoardKind && (a.boardPriority ?? 0) !== 0 && (
              <span
                className="rounded bg-[var(--color-bg)] px-1.5 py-0.5 font-mono text-[11px] text-[var(--color-text-dim)]"
                title={t('automationCard.priorityTitle')}
              >
                {t('automationCard.priority', { priority: a.boardPriority })}
              </span>
            )}
            {showContinue && (
              <span
                className="rounded bg-[var(--color-bg)] px-1.5 py-0.5 text-[11px] text-[var(--color-text-dim)]"
                title={t('automationCard.continueTitle')}
              >
                🧵 {t('automationCard.continue')}
              </span>
            )}
          </div>
          {a.name && (
            <div className="mt-0.5 truncate text-[13px] font-medium text-[var(--color-text)]">
              {a.name}
            </div>
          )}
          <div className="truncate text-xs text-[var(--color-text-dim)]">
            {t('common.targetArrow', { target: owner?.name ?? t('common.none') })}
          </div>
          <div
            className="mt-1 line-clamp-2 text-xs text-[var(--color-text-dim)]"
            title={a.promptTemplate}
          >
            {a.promptTemplate}
          </div>
        </div>

        {/* Edit (+ counter reset when maxed); deleting lives inside the popup. */}
        <div className="flex shrink-0 flex-col items-center gap-1.5">
          <CardAction icon={Pencil} label={t('common.edit')} onClick={onEdit} entityId={a.id} />
          <CardAction
            icon={Archive}
            label={t('common.archive')}
            onClick={onArchive}
            entityId={a.id}
          />
          {onPin && (
            <CardAction
              icon={a.pinned ? PinOff : Pin}
              label={a.pinned ? t('automationCard.unpin') : t('automationCard.pin')}
              onClick={onPin}
              entityId={a.id}
            />
          )}
          {maxed && (
            <CardAction
              icon={RotateCcw}
              label={t('automationCard.resetCounter')}
              onClick={onReset}
              entityId={a.id}
            />
          )}
        </div>
      </div>

      <div className="mt-1.5 flex flex-wrap items-center gap-x-2.5 gap-y-0.5 text-[11px] text-[var(--color-text-dim)]">
        <span className={maxed ? 'text-[var(--color-danger)]' : ''}>
          {t('automationCard.iterations')}: {a.iterationCount}
          {a.maxIterations > 0 ? ` / ${a.maxIterations}` : ' / ∞'}
          {maxed && ` (${t('automationCard.maxed')})`}
        </span>
        <span>{t('automationCard.cooldownSecondsValue', { seconds: a.cooldownSec })}</span>
        <span>
          {t('automationCard.last')}: {fmtTime(a.lastFiredAt)}
        </span>
        {a.expiresAt ? (
          <span className={expired ? 'text-[var(--color-danger)]' : ''}>
            {t('common.expiresAt')}: {fmtTime(a.expiresAt)}
            {expired && ` (${t('common.expired')})`}
          </span>
        ) : null}
      </div>
      {a.lastError && (
        <div className="mt-0.5 text-[11px] text-[var(--color-danger)]">
          {t('common.error')}: {a.lastError}
        </div>
      )}
      {/* Fire ledger (R5): every attempt with its outcome / skip reason. */}
      <AutomationFires automationId={a.id} refreshKey={a.lastFiredAt} />

      {isBoardKind ? (
        <div className="mt-1 text-[11px] text-[var(--color-text-dim)] opacity-80">
          {t('automationCard.boardNoLoop')}
        </div>
      ) : (
        <div className="mt-1">
          <span className="text-[10px] uppercase tracking-wide text-[var(--color-text-dim)] opacity-70">
            {t('automationCard.spawnTags')}
          </span>
          <TagEditor
            tags={a.spawnTags ?? [a.triggerTag]}
            onChange={onSpawnTags}
            placeholder={t('automationCard.spawnTagsPlaceholder')}
            className="py-1"
          />
        </div>
      )}
    </div>
  )
}
