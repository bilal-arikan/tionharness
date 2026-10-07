import type { CostBlock, DeltaStat } from '@/types'
import { InfoPopover } from '@/shared/components'
import { DeltaBadge } from './charts'
import { fmtUsd } from './chartFormat'
import { useTranslation } from 'react-i18next'

// CostSummary is the money row a CEO reads first: what today cost, month-to-date,
// the daily burn rate (with its period delta) and a naive month-end projection.
// Every figure is priced on the backend by the same rollup the Budget screen uses.
//
// The estimated flag (subscription providers like claude-cli, priced via an
// equivalent-API estimate) is shown once as a footnote rather than on every tile,
// so the "~" prefix stays legible where it matters.
export function CostSummary({ cost, costDelta }: { cost: CostBlock; costDelta: DeltaStat }) {
  const { t } = useTranslation('dashboard')
  const tiles = [
    { label: t('cost.today'), value: fmtUsd(cost.today, cost.estimated) },
    { label: t('cost.thisMonth'), value: fmtUsd(cost.month, cost.estimated) },
    {
      label: t('cost.dailyBurn'),
      value: fmtUsd(cost.burnRate, cost.estimated),
      // Up is the bad direction for spend, so the badge inverts its colours.
      delta: costDelta,
    },
    {
      label: t('cost.monthEndProjection'),
      value: fmtUsd(cost.projectedMonth, cost.estimated),
      hint: t('cost.atThisRate'),
    },
  ]

  return (
    <section className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
      <div className="mb-2 flex items-baseline gap-2">
        <span className="flex items-center gap-1 text-sm font-medium">
          💰 {t('cost.title')}
          {cost.estimated && <InfoPopover text={t('cost.estimatedNote')} />}
        </span>
        {cost.coolingWaste > 0 && (
          <span
            className="text-[11px] text-[var(--color-text-dim)]"
            title={t('cost.coolingWasteHint')}
          >
            {t('cost.coolingWaste', { value: fmtUsd(cost.coolingWaste, cost.estimated) })}
          </span>
        )}
      </div>
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
        {tiles.map((t) => (
          <div
            key={t.label}
            className="rounded-md border border-[var(--color-border)] bg-[var(--color-bg)] p-2.5"
          >
            <div className="text-[11px] text-[var(--color-text-dim)]">{t.label}</div>
            <div className="mt-0.5 flex items-baseline gap-1.5">
              <span className="text-xl font-semibold tabular-nums">{t.value}</span>
              {t.delta && <DeltaBadge d={t.delta} invert />}
            </div>
            {t.hint && (
              <div className="mt-0.5 text-[10px] text-[var(--color-text-dim)]">{t.hint}</div>
            )}
          </div>
        ))}
      </div>
    </section>
  )
}
