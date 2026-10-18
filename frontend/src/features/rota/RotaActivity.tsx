// Right-hand strip of the Rota skeleton: the workspace's latest automation
// fires and armed schedules from the lane store, newest first.
import { useTranslation } from 'react-i18next'
import type { LaneState } from '@/shared/lib/laneModel'
import { formatTime } from '@/shared/lib/intl'

interface Props {
  lanes: LaneState
}

const STRIP_LIMIT = 30

export function RotaActivity({ lanes }: Props) {
  const { t } = useTranslation('rota')
  const fires = lanes.fires.slice(-STRIP_LIMIT).reverse()
  return (
    <aside className="flex w-full shrink-0 flex-col gap-3 overflow-auto p-3 text-xs">
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
