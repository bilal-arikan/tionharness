import { useTranslation } from 'react-i18next'
import { Badge } from '@/shared/components'
import { formatDateTime } from '@/shared/lib/intl'
import type { Composition } from '@/types'
import { budgetPercent, sectionStateTone } from './notesHelpers'

interface Props {
  title: string
  composition?: Composition
  emptyLabel: string
}

// CompositionView shows one awareness delivery exactly as the agent received it:
// the meter line (what fit and what degraded), the per-section accounting and
// the full text. The text is rendered verbatim in <pre>; the SAME bytes went to
// the model, so a wrong or stale brief is visible rather than prettified away.
export function CompositionView({ title, composition, emptyLabel }: Props) {
  const { t } = useTranslation('notes')
  if (!composition) {
    return (
      <section className="rounded-lg border border-[var(--color-border)] p-3">
        <h3 className="text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
          {title}
        </h3>
        <p className="mt-1 text-xs text-[var(--color-text-dim)]">{emptyLabel}</p>
      </section>
    )
  }
  const pct = budgetPercent(composition.bytes, composition.budget)
  const over = composition.budget > 0 && composition.bytes > composition.budget
  return (
    <section className="flex flex-col gap-2 rounded-lg border border-[var(--color-border)] p-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
          {title}
        </h3>
        <div className="flex items-center gap-2 text-[11px] text-[var(--color-text-dim)]">
          <span className="tabular-nums">
            {composition.budget > 0
              ? t('context.bytesOfBudget', { bytes: composition.bytes, budget: composition.budget })
              : t('context.bytesUnlimited', { bytes: composition.bytes })}
          </span>
          {composition.cut && <Badge tone="danger">{t('context.cut')}</Badge>}
          {composition.at ? <span>{formatDateTime(composition.at * 1000)}</span> : null}
        </div>
      </div>
      {composition.budget > 0 && (
        <div className="h-1.5 w-full overflow-hidden rounded-full bg-[var(--color-surface-2)]">
          <div
            className={`h-full rounded-full ${over ? 'bg-[var(--color-danger)]' : 'bg-[var(--color-accent)]'}`}
            style={{ width: `${pct}%` }}
          />
        </div>
      )}
      {/* The meter is the last line of the delivered text, kept apart for emphasis. */}
      <div
        data-testid="composition-meter"
        className="rounded-md bg-[var(--color-accent-soft)] px-2 py-1 font-mono text-[11px] text-[var(--color-accent)]"
      >
        {composition.meter}
      </div>
      {composition.sections.length > 0 && (
        <table className="w-full text-left text-[11px]">
          <thead className="text-[var(--color-text-dim)]">
            <tr>
              <th className="py-0.5 pr-2 font-medium">{t('context.section')}</th>
              <th className="py-0.5 pr-2 font-medium">{t('context.bytes')}</th>
              <th className="py-0.5 pr-2 font-medium">{t('context.state')}</th>
              <th className="py-0.5 font-medium">{t('context.priority')}</th>
            </tr>
          </thead>
          <tbody>
            {composition.sections.map((s) => (
              <tr key={s.key} className="border-t border-[var(--color-border)]">
                <td className="py-0.5 pr-2 font-mono">{s.key}</td>
                <td className="py-0.5 pr-2 tabular-nums">{s.bytes}</td>
                <td className="py-0.5 pr-2">
                  <Badge tone={sectionStateTone(s.state)}>{t(`context.states.${s.state}`)}</Badge>
                </td>
                <td className="py-0.5 tabular-nums">{s.priority}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {((composition.degraded?.length ?? 0) > 0 || (composition.dropped?.length ?? 0) > 0) && (
        <p className="text-[11px] text-[var(--color-text-dim)]">
          {(composition.degraded?.length ?? 0) > 0 &&
            t('context.degraded', { keys: composition.degraded!.join(', ') })}
          {(composition.degraded?.length ?? 0) > 0 && (composition.dropped?.length ?? 0) > 0
            ? ' · '
            : ''}
          {(composition.dropped?.length ?? 0) > 0 &&
            t('context.dropped', { keys: composition.dropped!.join(', ') })}
        </p>
      )}
      <pre className="max-h-96 overflow-auto whitespace-pre-wrap rounded-md bg-[var(--color-surface-2)] p-3 text-xs leading-relaxed text-[var(--color-text)]">
        {composition.text}
      </pre>
    </section>
  )
}
