// Right-hand strip of the Rota skeleton: latest automation fires and
// coordination facts from the lane store, newest first.
import type { LaneState } from '@/shared/lib/laneModel'

interface Props {
  lanes: LaneState
}

const STRIP_LIMIT = 30

export function RotaActivity({ lanes }: Props) {
  const fires = lanes.fires.slice(-STRIP_LIMIT).reverse()
  const activity = lanes.activity.slice(-STRIP_LIMIT).reverse()
  return (
    <aside className="hidden w-64 shrink-0 flex-col gap-3 overflow-auto border-l border-[var(--color-border)] p-3 text-xs md:flex">
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
