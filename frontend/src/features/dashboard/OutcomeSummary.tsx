import type { OutcomeBlock } from '@/types'
import { DayBars } from './charts'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { count, percent } from '@/shared/lib/format'

// fmtDuration renders a second span as the coarsest readable unit, matching the
// view layer's dur() idiom (31s, 2m, 3sa, 4g). Used for cycle time.
function duration(
  t: TFunction<'dashboard'>,
  key: 'duration.seconds' | 'duration.minutes' | 'duration.hours' | 'duration.days',
  value: number,
): string {
  return t(key, { count: value, value: count(value) })
}

function fmtDuration(sec: number, t: TFunction<'dashboard'>): string {
  if (sec <= 0) return '—'
  if (sec < 60) return duration(t, 'duration.seconds', Math.round(sec))
  if (sec < 3600) return duration(t, 'duration.minutes', Math.round(sec / 60))
  if (sec < 86400) return duration(t, 'duration.hours', Math.round(sec / 3600))
  return duration(t, 'duration.days', Math.round(sec / 86400))
}

// OutcomeSummary is the completion side of the overview: throughput (cards
// finishing per day), how long a card takes end to end, and the flow-run success
// rate. The volume trends say how much STARTED; this says how much FINISHED.
//
// Each chip states its own "no data" case in words: a rate off zero closed runs
// is undefined, and a cycle time with no finished card is not "0 seconds".
export function OutcomeSummary({ o }: { o: OutcomeBlock }) {
  const { t } = useTranslation('dashboard')
  const rate = o.runSuccessRate
  const chips = [
    {
      label: t('outcomes.completedCards'),
      value: count(o.cardsDoneWindow),
      hint: t('outcomes.inWindow'),
    },
    {
      label: t('outcomes.averageCompletion'),
      value: o.cardsDoneWindow > 0 ? fmtDuration(o.cycleTimeAvgSec, t) : '—',
      hint: o.cardsDoneWindow > 0 ? t('outcomes.createdToDone') : t('outcomes.noCompletedCards'),
    },
    {
      label: t('outcomes.runSuccessRate'),
      value: rate == null ? '—' : percent(rate),
      hint:
        rate == null
          ? t('outcomes.noClosedRuns')
          : t('outcomes.closedRuns', {
              count: o.runsClosedWindow,
              value: count(o.runsClosedWindow),
            }),
      tone: rate != null && rate < 0.8 ? 'warn' : 'normal',
    },
  ]

  return (
    <section className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
      <div className="mb-2 text-sm font-medium">✅ {t('outcomes.title')}</div>
      <div className="mb-3 grid grid-cols-3 gap-2">
        {chips.map((c) => (
          <div
            key={c.label}
            className="rounded-md border border-[var(--color-border)] bg-[var(--color-bg)] p-2.5"
          >
            <div className="text-[11px] text-[var(--color-text-dim)]">{c.label}</div>
            <div
              className={`mt-0.5 text-xl font-semibold tabular-nums ${
                c.tone === 'warn' ? 'text-[var(--color-danger)]' : ''
              }`}
            >
              {c.value}
            </div>
            <div className="mt-0.5 truncate text-[10px] text-[var(--color-text-dim)]">{c.hint}</div>
          </div>
        ))}
      </div>
      <DayBars
        points={o.velocityByDay}
        label={t('outcomes.cardsPerDay')}
        color="var(--color-success)"
      />
    </section>
  )
}
