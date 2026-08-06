import { forwardRef } from 'react'
import { Play, Hourglass, Pencil, Workflow } from 'lucide-react'
import type { Agent, Flow, Schedule } from '@/types'
import { AgentAvatar } from '@/shared/components/agents/AgentAvatar'
import { TagEditor } from '@/shared/components'
import { normalizeAvatar } from '@/shared/lib/avatar'
import { CardAction } from './pickers'
import { COLUMN_ACCENT } from './automationMeta'
import { fmtTime, isPast } from './timeUtils'

interface Props {
  schedule: Schedule
  agents: Agent[]
  flows: Flow[]
  highlighted: boolean
  running: boolean
  onToggle: () => void
  onRunNow: () => void
  onEdit: () => void
  onTags: (tags: string[]) => void
}

// ScheduleCard is one cron rule rendered as a board card: enable toggle + target
// avatar on the left, cron/prompt/run info in the body, run + edit actions on the
// right. Deleting is intentionally NOT here — it lives inside the edit popup so a
// mis-click on a dense lane cannot destroy a rule.
export const ScheduleCard = forwardRef<HTMLDivElement, Props>(function ScheduleCard(
  { schedule: s, agents, flows, highlighted, running, onToggle, onRunNow, onEdit, onTags },
  ref,
) {
  const flow = flows.find((f) => f.id === s.flowId)
  const flowIcon = normalizeAvatar(flow?.emoji)
  const owner = agents.find((a) => a.id === s.agentId)
  const expired = isPast(s.expiresAt)

  return (
    <div
      ref={ref}
      data-testid="schedule-row"
      data-schedule-id={s.id}
      className={`rounded-lg border border-l-4 bg-[var(--color-surface)] px-2.5 py-2 text-sm transition ${
        highlighted
          ? 'border-[var(--color-accent)] ring-2 ring-[var(--color-accent)]'
          : 'border-[var(--color-border)]'
      }`}
      style={{ borderLeftColor: COLUMN_ACCENT.schedules }}
    >
      <div className="flex items-start gap-2">
        <div className="flex shrink-0 flex-col items-center gap-1.5">
          <button
            data-testid="schedule-enable-toggle"
            data-schedule-id={s.id}
            type="button"
            role="switch"
            aria-checked={s.enabled}
            aria-label={s.enabled ? 'Etkin' : 'Pasif'}
            onClick={onToggle}
            className={`h-4 w-8 rounded-full transition ${
              s.enabled ? 'bg-[var(--color-accent)]' : 'bg-[var(--color-border)]'
            }`}
            title={s.enabled ? 'Etkin' : 'Pasif'}
          >
            <span
              className={`block h-4 w-4 rounded-full bg-white transition ${s.enabled ? 'translate-x-4' : ''}`}
            />
          </button>
          {s.flowId ? (
            <span
              className="flex h-7 w-7 items-center justify-center rounded-full bg-[var(--color-accent-soft)] text-[var(--color-accent)]"
              title="Akış tabanlı zamanlama"
            >
              {flowIcon ? (
                <span className="text-base leading-none">{flowIcon}</span>
              ) : (
                <Workflow size={15} />
              )}
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
          <div className="font-mono text-[13px] text-[var(--color-accent)]">{s.cronExpr}</div>
          <div className="truncate text-xs text-[var(--color-text-dim)]">
            → {s.flowId ? `${flowIcon ?? '🔀'} ${flow?.name ?? s.flowId}` : (owner?.name ?? '—')}
          </div>
          {(s.prompt || !s.flowId) && (
            <div
              className="mt-1 line-clamp-2 text-xs text-[var(--color-text-dim)]"
              title={s.prompt}
            >
              {s.flowId ? 'Girdi' : 'Prompt'}: {s.prompt}
            </div>
          )}
        </div>

        {/* Run + edit only; deleting lives inside the edit popup. */}
        <div className="flex shrink-0 flex-col items-center gap-1.5">
          <CardAction
            icon={running ? Hourglass : Play}
            label="Şimdi çalıştır"
            tone="success"
            disabled={running}
            onClick={onRunNow}
            testId="schedule-run-now"
            entityId={s.id}
          />
          <CardAction
            icon={Pencil}
            label="Düzenle"
            onClick={onEdit}
            testId="schedule-edit"
            entityId={s.id}
          />
        </div>
      </div>

      <div className="mt-1.5 space-y-0.5 text-[11px] text-[var(--color-text-dim)]">
        <div>
          Sonraki: {fmtTime(s.nextRunAt)} · Son:{' '}
          {s.lastDeliveryStatus ? (
            <span
              className={
                s.lastDeliveryStatus === 'success'
                  ? 'text-[var(--color-success)]'
                  : 'text-[var(--color-danger)]'
              }
            >
              {s.lastDeliveryStatus} {fmtTime(s.lastRunAt)}
            </span>
          ) : (
            '—'
          )}
        </div>
        {s.lastDeliveryError && (
          <div className="text-[var(--color-danger)]">Hata: {s.lastDeliveryError}</div>
        )}
        {s.expiresAt ? (
          <div className={expired ? 'text-[var(--color-danger)]' : ''}>
            Son tarih: {fmtTime(s.expiresAt)}
            {expired && ' (süresi doldu)'}
          </div>
        ) : null}
      </div>

      <div className="mt-1">
        <TagEditor tags={s.tags ?? []} onChange={onTags} className="py-1" />
      </div>
    </div>
  )
})
