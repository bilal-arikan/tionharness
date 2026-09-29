import type { ShellOptimization } from '@/types'
import { useTranslation } from 'react-i18next'

interface Props {
  optimizer: ShellOptimization
}

// OptimizerChip marks a tool step whose output was shrunk by an external token
// optimizer before the model read it. Without it the rewrite is invisible: the
// agent sees sqz's abbreviated text (an inline legend + «A1» placeholders) and
// the user, reading the same card, has no way to tell that from truncation.
//
// 'sqz' reports exact token counts, so the chip shows the real reduction and the
// before/after pair on hover. 'rtk' wraps the command upstream of us — there is
// no before/after to measure, so it shows the name only rather than a made-up
// percentage.
export function OptimizerChip({ optimizer }: Props) {
  const { t } = useTranslation('chatStatus')
  const { kind, inTokens, outTokens, dedup, command, degraded } = optimizer
  const measured =
    typeof inTokens === 'number' &&
    typeof outTokens === 'number' &&
    inTokens > 0 &&
    outTokens < inTokens
  const percent = measured ? Math.round(((inTokens - outTokens) / inTokens) * 100) : 0

  // A rewritten command that FAILED is the one case where the optimizer may be
  // actively misleading, so it takes precedence over any saving figure — a "−%93"
  // next to a hidden error would be celebrating the wrong thing. Then dedup, which
  // a reader would otherwise file as a bug (the command printed plenty, the card
  // shows one "§ref:…§" line).
  const title = degraded
    ? t('optimizer.degradedTitle', { kind }) +
      (command ? `\n\n${t('optimizer.command', { command })}` : '')
    : dedup
      ? t('optimizer.deduplicatedTitle', { kind })
      : (measured
          ? t('optimizer.measuredTitle', { kind, input: inTokens, output: outTokens, percent })
          : t('optimizer.optimizedTitle', { kind })) +
        (command ? `\n\n${t('optimizer.command', { command })}` : '')

  return (
    <span
      title={title}
      className={`shrink-0 rounded px-1.5 py-px font-mono text-[10px] ${
        degraded
          ? 'bg-[var(--color-surface-2)] text-[var(--color-warning)]'
          : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)]'
      }`}
    >
      {kind}
      {degraded ? (
        <span className="ml-1">{t('optimizer.degradedBadge')}</span>
      ) : dedup ? (
        <span className="ml-1 text-[var(--color-success)]">{t('optimizer.deduplicatedBadge')}</span>
      ) : (
        measured && (
          <span className="ml-1 text-[var(--color-success)]">
            {t('optimizer.savingBadge', { percent })}
          </span>
        )
      )}
    </span>
  )
}
