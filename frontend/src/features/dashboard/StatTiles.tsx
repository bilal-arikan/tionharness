import type { DashboardCounters } from '@/types'
import { useTranslation } from 'react-i18next'
import { count } from '@/shared/lib/format'

interface Tile {
  label: string
  value: number
  // hint is the secondary line: context that turns a bare number into a state.
  hint?: string
  // tone colours the value when the number itself is the warning.
  tone?: 'normal' | 'warn' | 'danger'
}

// StatTiles is the headline row. Only counts that a reader can ACT on are here —
// a tile that never changes teaches people to stop looking at the row.
export function StatTiles({ c }: { c: DashboardCounters }) {
  const { t } = useTranslation('dashboard')
  const tiles: Tile[] = [
    { label: t('stats.agents'), value: c.agents },
    {
      label: t('stats.activeSessions'),
      value: c.sessionsActive,
      hint: t('stats.sessionsHint', {
        total: count(c.sessions),
        archived:
          c.sessionsArchived > 0
            ? t('stats.archived', {
                count: c.sessionsArchived,
                value: count(c.sessionsArchived),
              })
            : '',
      }),
    },
    {
      label: t('stats.stuckSessions'),
      value: c.sessionsStuck,
      // Zero is the healthy value here, so it must not shout.
      tone: c.sessionsStuck > 0 ? 'danger' : 'normal',
      hint: c.sessionsStuck > 0 ? 'StuckTurns > 0' : t('stats.clear'),
    },
    {
      label: t('stats.openCards'),
      value: c.tasksOpen,
      hint: t('stats.total', { count: c.tasks, value: count(c.tasks) }),
    },
    {
      label: t('stats.runningRuns'),
      value: c.runsRunning,
      tone: c.runsRunning > 0 ? 'warn' : 'normal',
      hint:
        c.runsWaiting > 0
          ? t('stats.waitingForInput', {
              count: c.runsWaiting,
              value: count(c.runsWaiting),
            })
          : t('stats.total', { count: c.runs, value: count(c.runs) }),
    },
    {
      label: t('stats.failedRuns'),
      value: c.runsFailed,
      tone: c.runsFailed > 0 ? 'danger' : 'normal',
      hint: c.runsFailed > 0 ? t('stats.needsReview') : t('stats.clear'),
    },
  ]

  // One bordered panel split into cells (gap-px over a border-coloured fill draws
  // the dividers, and every breakpoint's column count divides 6 so rows stay full).
  return (
    <div className="grid grid-cols-2 gap-px overflow-hidden rounded-lg border border-[var(--color-border)] bg-[var(--color-border)] sm:grid-cols-3 lg:grid-cols-6 square:grid-cols-3">
      {tiles.map((t) => (
        <div key={t.label} className="min-w-0 bg-[var(--color-surface)] px-3 py-2">
          <div className="truncate text-[11px] text-[var(--color-text-dim)]">{t.label}</div>
          <div className="flex min-w-0 items-baseline gap-1.5">
            <span
              className={`text-lg font-semibold leading-tight tabular-nums ${
                t.tone === 'danger'
                  ? 'text-[var(--color-danger)]'
                  : t.tone === 'warn'
                    ? 'text-[var(--color-accent)]'
                    : ''
              }`}
            >
              {count(t.value)}
            </span>
            {t.hint && (
              <span
                className="min-w-0 truncate text-[10px] text-[var(--color-text-dim)]"
                title={t.hint}
              >
                {t.hint}
              </span>
            )}
          </div>
        </div>
      ))}
    </div>
  )
}
