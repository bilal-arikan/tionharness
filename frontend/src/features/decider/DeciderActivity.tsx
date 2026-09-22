// The decision ledger's most recent entries: which authority decided what, with
// which model, how sure it was, what it was compared with (the existing logic
// in shadow mode, the answering model for a challenger), latency and cost.
import { useTranslation } from 'react-i18next'
import type { DeciderAuthority, DeciderModelInstance, DeciderRecord } from '@/types/decider'
import { Badge, SectionHead } from '@/shared/components'
import { formatTime } from '@/shared/lib/intl'
import { percent, usd } from '@/shared/lib/format'
import { modelLabel } from './deciderModel'

interface Props {
  records: DeciderRecord[]
  days: number
  models: DeciderModelInstance[]
  authorities: DeciderAuthority[]
}

const MODE_TONE = { off: 'muted', shadow: 'accent', on: 'success' } as const

export function DeciderActivity({ records, days, models, authorities }: Props) {
  const { t } = useTranslation('decider')
  const authorityLabel = (id: string) =>
    t(`authority.${id}.label`, { defaultValue: authorities.find((a) => a.id === id)?.label ?? id })
  return (
    <section className="space-y-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
      <SectionHead>
        {t('recent.title')} · {t('stats.title', { days })}
      </SectionHead>
      {records.length === 0 ? (
        <p className="text-xs text-[var(--color-text-dim)]">{t('recent.empty')}</p>
      ) : (
        <ul className="divide-y divide-[var(--color-border)] text-xs" data-testid="decider-recent">
          {records.map((r, i) => (
            <li key={`${r.at}-${i}`} className="flex flex-wrap items-center gap-x-2 gap-y-1 py-1.5">
              <span className="tabular-nums text-[var(--color-text-dim)]">{formatTime(r.at)}</span>
              <span className="font-medium">{authorityLabel(r.authority)}</span>
              <Badge tone={MODE_TONE[r.mode] ?? 'muted'}>{t(`modeLabel.${r.mode}`)}</Badge>
              {r.role === 'challenger' && <Badge tone="warning">{t('recent.challenger')}</Badge>}
              {r.fallback && <Badge tone="warning">{t('recent.fallback')}</Badge>}
              {r.instance && (
                <span className="text-[var(--color-text-dim)]">
                  {modelLabel(models, r.instance)}
                </span>
              )}
              {r.error ? (
                <span className="text-[var(--color-danger)]">
                  {t('recent.failed', { error: r.error })}
                </span>
              ) : (
                <>
                  <code className="rounded bg-[var(--color-surface-2)] px-1">{r.outcome}</code>
                  {r.strength !== undefined && (
                    <span className="tabular-nums text-[var(--color-text-dim)]">
                      {percent(r.strength)}
                    </span>
                  )}
                  {r.baseline && r.baseline !== r.outcome && (
                    <span className="text-[var(--color-warning)]">
                      {t('recent.vs', { baseline: r.baseline })}
                    </span>
                  )}
                  {r.applied && <Badge tone="success">{t('recent.applied')}</Badge>}
                </>
              )}
              <span className="ms-auto tabular-nums text-[var(--color-text-dim)]">
                {r.latencyMs ? `${r.latencyMs} ms` : ''}
                {r.costUsd ? ` · ${usd(r.costUsd)}` : ''}
              </span>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
