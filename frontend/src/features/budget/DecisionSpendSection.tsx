import { Coins } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { InfoPopover } from '@/shared/components'
import { formatDateTime, numberFormat } from '@/shared/lib/intl'
import type { DecisionSpendReport, DecisionSpendTotals } from '@/types/decisionSpend'

const PROVIDER_LABELS: Record<string, string> = {
  openrouter: 'OpenRouter',
  typesafe: 'TypeSafe',
  local: 'Local',
}

export function DecisionSpendSection({ report }: { report: DecisionSpendReport }) {
  const { t } = useTranslation('budget')
  const currency = numberFormat({
    style: 'currency',
    currency: 'USD',
    currencyDisplay: 'narrowSymbol',
    minimumFractionDigits: 2,
    maximumFractionDigits: 8,
  })
  const count = numberFormat({ maximumFractionDigits: 0 })
  const money = (value: number) => {
    const amount = Number.isFinite(value) ? Math.max(0, value) : 0
    return amount > 0 && amount < 0.00000001
      ? t('decisionSpend.tinyCost', {
          defaultValue: '< {{value}}',
          value: currency.format(0.00000001),
        })
      : currency.format(amount)
  }
  const callsTokens = (totals: DecisionSpendTotals) =>
    t('decisionSpend.callsTokens', {
      defaultValue: '{{calls}} calls · {{input}} input / {{output}} output tokens',
      calls: count.format(totals.calls),
      input: count.format(totals.inputTokens),
      output: count.format(totals.outputTokens),
    })
  const unknown = (totals: DecisionSpendTotals) =>
    t('decisionSpend.unknown', {
      defaultValue: 'Cost unavailable for {{count}} calls',
      count: totals.unknownCostCalls,
    })
  const summary = (label: string, totals: DecisionSpendTotals) => (
    <div className="min-w-0 rounded-lg border border-[var(--color-border)] p-3">
      <h3 className="text-[11px] text-[var(--color-text-dim)]">{label}</h3>
      <div className="mt-1 break-words text-lg font-semibold text-[var(--color-text)]">
        {money(totals.costUSD)}
      </div>
      <p className="mt-1 text-[11px] text-[var(--color-text-dim)]">{callsTokens(totals)}</p>
      <dl className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-[11px]">
        <div>
          <dt className="inline text-[var(--color-text-dim)]">
            {t('decisionSpend.reported', { defaultValue: 'Reported' })}:{' '}
          </dt>
          <dd className="inline text-[var(--color-text)]">{money(totals.reportedCostUSD)}</dd>
        </div>
        <div>
          <dt className="inline text-[var(--color-text-dim)]">
            {t('decisionSpend.estimated', { defaultValue: 'Estimated' })}:{' '}
          </dt>
          <dd className="inline text-[var(--color-text)]">{money(totals.estimatedCostUSD)}</dd>
        </div>
      </dl>
      {totals.unknownCostCalls > 0 && (
        <p className="mt-2 text-[11px] text-[var(--color-warning)]">{unknown(totals)}</p>
      )}
    </div>
  )

  return (
    <section className="mb-5 min-w-0 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] [overflow-wrap:anywhere]">
      <div className="flex items-center gap-2 border-b border-[var(--color-border)] px-4 py-2.5">
        <Coins size={15} className="shrink-0 text-[var(--color-accent)]" aria-hidden="true" />
        <h2 className="text-sm font-medium text-[var(--color-text)]">
          {t('decisionSpend.title', { defaultValue: 'Decision model spend' })}
        </h2>
        <InfoPopover
          text={t('decisionSpend.scope', {
            defaultValue:
              'Across all workspaces. This separate view is not added to workspace totals; some calls already appear in agent usage.',
          })}
        />
      </div>
      <div className="min-w-0 space-y-3 p-3 sm:p-4">
        {report.startedAt > 0 && (
          <p className="text-[11px] text-[var(--color-text-dim)]">
            {t('decisionSpend.startedAt', {
              defaultValue: 'Tracking since {{date}}',
              date: formatDateTime(report.startedAt),
            })}
          </p>
        )}
        {report.storageError && (
          <p role="alert" className="text-xs text-[var(--color-danger)]">
            {t('decisionSpend.storageError', {
              defaultValue: 'Spend storage is unavailable. These totals may be incomplete.',
            })}
          </p>
        )}
        <div className="grid min-w-0 grid-cols-1 gap-3 sm:grid-cols-2">
          {summary(t('decisionSpend.today', { defaultValue: 'Today' }), report.today)}
          {summary(
            t('decisionSpend.window', { defaultValue: 'Last {{count}} days', count: report.days }),
            report.totals,
          )}
        </div>
        <p className="text-[11px] text-[var(--color-text-dim)]">
          {t('decisionSpend.roles', {
            defaultValue: '{{primary}} primary · {{challenger}} challenger · {{tests}} test calls',
            primary: count.format(report.totals.primaryCalls),
            challenger: count.format(report.totals.challengerCalls),
            tests: count.format(report.totals.testCalls),
          })}
        </p>
        {report.models.length === 0 ? (
          <p className="text-xs text-[var(--color-text-dim)]">
            {t('decisionSpend.empty', { defaultValue: 'No decision model calls in this period.' })}
          </p>
        ) : (
          <div className="min-w-0 space-y-2">
            <div className="hidden grid-cols-[minmax(0,1fr)_auto_auto] gap-4 px-3 text-[11px] text-[var(--color-text-dim)] sm:grid">
              <span>{t('decisionSpend.providerModel', { defaultValue: 'Provider / model' })}</span>
              <span>{t('decisionSpend.calls', { defaultValue: 'Calls' })}</span>
              <span>{t('decisionSpend.cost', { defaultValue: 'Cost' })}</span>
            </div>
            {report.models.map((model) => (
              <div
                key={`${model.provider}:${model.model}`}
                className="grid min-w-0 grid-cols-1 gap-2 rounded-lg border border-[var(--color-border)] p-3 sm:grid-cols-[minmax(0,1fr)_auto_auto] sm:gap-4"
              >
                <div className="min-w-0 text-xs">
                  <div className="text-[var(--color-text)]">
                    {PROVIDER_LABELS[model.provider] ?? (model.provider || 'unknown')} ·{' '}
                    {model.model}
                  </div>
                  <p className="mt-1 text-[11px] text-[var(--color-text-dim)]">
                    {callsTokens(model)}
                  </p>
                  {model.unknownCostCalls > 0 && (
                    <p className="mt-1 text-[11px] text-[var(--color-warning)]">{unknown(model)}</p>
                  )}
                </div>
                <div className="text-xs text-[var(--color-text)]">
                  <span className="mr-1 text-[var(--color-text-dim)] sm:hidden">
                    {t('decisionSpend.calls', { defaultValue: 'Calls' })}:{' '}
                  </span>
                  {count.format(model.calls)}
                </div>
                <div className="text-xs font-medium text-[var(--color-text)]">
                  <span className="mr-1 font-normal text-[var(--color-text-dim)] sm:hidden">
                    {t('decisionSpend.cost', { defaultValue: 'Cost' })}:{' '}
                  </span>
                  {money(model.costUSD)}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </section>
  )
}
