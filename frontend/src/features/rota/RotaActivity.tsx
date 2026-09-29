// Right-hand strip of the Rota skeleton: the workspace's trajectories, latest
// automation fires and coordination facts from the lane store, newest first.
import { Badge } from '@/shared/components'
import { useTranslation } from 'react-i18next'
import type { LaneState } from '@/shared/lib/laneModel'
import type { TrajectoryStatus } from '@/types/trajectory'
import { STATUS_TONE, trajectoryStatusLabel } from './trajectoryStatus'
import { formatTime } from '@/shared/lib/intl'

interface Props {
  lanes: LaneState
  onOpenTrajectory?: (trajectoryId: string) => void
}

const STRIP_LIMIT = 30

export function RotaActivity({ lanes, onOpenTrajectory }: Props) {
  const { t } = useTranslation('rota')
  const fires = lanes.fires.slice(-STRIP_LIMIT).reverse()
  const activity = lanes.activity.slice(-STRIP_LIMIT).reverse()
  const trajectories = [...lanes.trajectories.values()]
    .filter((t) => t.op !== 'delete')
    .sort((a, b) => (b.updatedAt ?? 0) - (a.updatedAt ?? 0))
    .slice(0, STRIP_LIMIT)
  return (
    <aside className="flex w-full shrink-0 flex-col gap-3 overflow-auto p-3 text-xs">
      <section>
        <h3 className="mb-1 font-medium">{t('activity.trajectories')}</h3>
        {trajectories.length === 0 ? (
          <p className="text-[var(--color-text-dim)]">{t('activity.noTrajectories')}</p>
        ) : (
          <ul className="flex flex-col gap-1">
            {trajectories.map((trajectory) => {
              const root = lanes.sessions.get(trajectory.rootSessionId)
              const status = (trajectory.status ?? 'planned') as TrajectoryStatus
              return (
                <li key={trajectory.trajectoryId}>
                  <button
                    type="button"
                    onClick={() => onOpenTrajectory?.(trajectory.trajectoryId)}
                    className="flex w-full items-center gap-1.5 rounded px-1.5 py-1 text-left hover:bg-[var(--color-surface-2)]"
                    title={t('activity.trajectoryTitle', {
                      id: trajectory.trajectoryId,
                      template: trajectory.templateRef || t('common.unplanned'),
                      revision: trajectory.revision,
                      count: trajectory.nodeCount,
                    })}
                  >
                    <span className="text-[var(--color-accent)]">◈</span>
                    <span className="min-w-0 flex-1 truncate">
                      {root?.title || trajectory.rootSessionId}
                      <span className="text-[var(--color-text-dim)]">
                        {' '}
                        · {trajectory.templateRef || t('common.unplanned')}
                      </span>
                    </span>
                    <Badge tone={STATUS_TONE[status] ?? 'muted'}>
                      {trajectoryStatusLabel(status)}
                    </Badge>
                  </button>
                </li>
              )
            })}
          </ul>
        )}
      </section>
      <section>
        <h3 className="mb-1 font-medium">{t('activity.triggers')}</h3>
        {fires.length === 0 ? (
          <p className="text-[var(--color-text-dim)]">{t('activity.noTriggers')}</p>
        ) : (
          <ul className="flex flex-col gap-0.5">
            {fires.map((f) => (
              <li key={f.seq} className="truncate" title={f.reason}>
                <span
                  className={
                    f.outcome === 'fired' ? 'text-emerald-600' : 'text-[var(--color-text-dim)]'
                  }
                >
                  {f.outcome === 'fired' ? '⚡' : '↷'}
                </span>{' '}
                {f.name || f.automationId} · {f.triggerKind}
                {f.reason ? ` · ${f.reason}` : ''}
              </li>
            ))}
          </ul>
        )}
      </section>
      <section>
        <h3 className="mb-1 font-medium">{t('activity.coordination')}</h3>
        {activity.length === 0 ? (
          <p className="text-[var(--color-text-dim)]">{t('activity.noEvents')}</p>
        ) : (
          <ul className="flex flex-col gap-0.5">
            {activity.map((a) => (
              <li key={a.seq} className="truncate" title={a.reason}>
                {a.kind}
                {a.phase ? `:${a.phase}` : ''} · {a.coordinatorId}
                {a.workerId ? ` → ${a.workerId}` : ''}
                {a.status ? ` · ${a.status}` : ''}
              </li>
            ))}
          </ul>
        )}
      </section>
      <section>
        <h3 className="mb-1 font-medium">{t('activity.armedSchedules')}</h3>
        {lanes.armed.size === 0 ? (
          <p className="text-[var(--color-text-dim)]">{t('activity.noArmedSchedules')}</p>
        ) : (
          <ul className="flex flex-col gap-0.5">
            {[...lanes.armed.values()].map((s) => (
              <li key={s.scheduleId} className="truncate">
                ⏰ {s.name || s.scheduleId} · {formatTime(s.fireAt * 1000)}
              </li>
            ))}
          </ul>
        )}
      </section>
    </aside>
  )
}
