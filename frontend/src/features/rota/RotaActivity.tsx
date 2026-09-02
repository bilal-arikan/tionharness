// Right-hand strip of the Rota skeleton: the workspace's trajectories, latest
// automation fires and coordination facts from the lane store, newest first.
import { Badge } from '@/shared/components'
import type { LaneState } from '@/shared/lib/laneModel'
import type { TrajectoryStatus } from '@/types/trajectory'
import { STATUS_LABEL, STATUS_TONE } from './trajectoryStatus'

interface Props {
  lanes: LaneState
  onOpenTrajectory?: (trajectoryId: string) => void
}

const STRIP_LIMIT = 30

export function RotaActivity({ lanes, onOpenTrajectory }: Props) {
  const fires = lanes.fires.slice(-STRIP_LIMIT).reverse()
  const activity = lanes.activity.slice(-STRIP_LIMIT).reverse()
  const trajectories = [...lanes.trajectories.values()]
    .filter((t) => t.op !== 'delete')
    .sort((a, b) => (b.updatedAt ?? 0) - (a.updatedAt ?? 0))
    .slice(0, STRIP_LIMIT)
  return (
    <aside className="flex w-full shrink-0 flex-col gap-3 overflow-auto p-3 text-xs">
      <section>
        <h3 className="mb-1 font-medium">Rotalar</h3>
        {trajectories.length === 0 ? (
          <p className="text-[var(--color-text-dim)]">
            Henüz rota yok. Bir koordinatör worker açtığında veya reçeteyle başladığında burada
            belirir.
          </p>
        ) : (
          <ul className="flex flex-col gap-1">
            {trajectories.map((t) => {
              const root = lanes.sessions.get(t.rootSessionId)
              const status = (t.status ?? 'planned') as TrajectoryStatus
              return (
                <li key={t.trajectoryId}>
                  <button
                    type="button"
                    onClick={() => onOpenTrajectory?.(t.trajectoryId)}
                    className="flex w-full items-center gap-1.5 rounded px-1.5 py-1 text-left hover:bg-[var(--color-surface-2)]"
                    title={`${t.trajectoryId} · ${t.templateRef || 'plansız'} · rev ${t.revision} · ${t.nodeCount} düğüm`}
                  >
                    <span className="text-[var(--color-accent)]">◈</span>
                    <span className="min-w-0 flex-1 truncate">
                      {root?.title || t.rootSessionId}
                      <span className="text-[var(--color-text-dim)]">
                        {' '}
                        · {t.templateRef || 'plansız'}
                      </span>
                    </span>
                    <Badge tone={STATUS_TONE[status] ?? 'muted'}>
                      {STATUS_LABEL[status] ?? status}
                    </Badge>
                  </button>
                </li>
              )
            })}
          </ul>
        )}
      </section>
      <section>
        <h3 className="mb-1 font-medium">Tetikler</h3>
        {fires.length === 0 ? (
          <p className="text-[var(--color-text-dim)]">Henüz tetik yok.</p>
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
        <h3 className="mb-1 font-medium">Koordinasyon</h3>
        {activity.length === 0 ? (
          <p className="text-[var(--color-text-dim)]">Henüz olay yok.</p>
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
        <h3 className="mb-1 font-medium">Kurulu zamanlayıcılar</h3>
        {lanes.armed.size === 0 ? (
          <p className="text-[var(--color-text-dim)]">Kurulu zamanlayıcı yok.</p>
        ) : (
          <ul className="flex flex-col gap-0.5">
            {[...lanes.armed.values()].map((s) => (
              <li key={s.scheduleId} className="truncate">
                ⏰ {s.name || s.scheduleId} · {new Date(s.fireAt * 1000).toLocaleTimeString()}
              </li>
            ))}
          </ul>
        )}
      </section>
    </aside>
  )
}
