// Rota toolbar: the idle-lane window. Live lanes always stay; idle ones drop
// out of the canvas once their last activity is older than the window.
import { useTranslation } from 'react-i18next'
export type IdleCutoff = 3600 | 21600 | 86400 | 259200 | 0

const OPTIONS: IdleCutoff[] = [3600, 21600, 86400, 259200, 0]

interface Props {
  cutoff: IdleCutoff
  onCutoff: (v: IdleCutoff) => void
  // Collapse stretches of the window where no lane did anything (default on).
  collapseGaps: boolean
  onCollapseGaps: (v: boolean) => void
  // Spend time on a log scale so long sessions stop eating the panel (default on).
  normalizeBars: boolean
  onNormalizeBars: (v: boolean) => void
}

export function RotaToolbar({
  cutoff,
  onCutoff,
  collapseGaps,
  onCollapseGaps,
  normalizeBars,
  onNormalizeBars,
}: Props) {
  const { t } = useTranslation('rota')
  return (
    <>
      <span className="flex items-center gap-1" title={t('toolbar.windowTitle')}>
        <span className="text-[var(--color-text-dim)]">{t('toolbar.window')}</span>
        {OPTIONS.map((value) => (
          <button
            key={value}
            type="button"
            onClick={() => onCutoff(value)}
            className={`rounded px-1.5 py-0.5 ${
              cutoff === value
                ? 'bg-[var(--color-accent-soft)] text-[var(--color-text)]'
                : 'text-[var(--color-text-dim)] hover:text-[var(--color-text)]'
            }`}
          >
            {t(`toolbar.windowOptions.${value}`)}
          </button>
        ))}
      </span>
      <button
        type="button"
        role="switch"
        aria-checked={collapseGaps}
        onClick={() => onCollapseGaps(!collapseGaps)}
        title={t('toolbar.collapseGapsTitle')}
        className={`rounded px-1.5 py-0.5 ${
          collapseGaps
            ? 'bg-[var(--color-accent-soft)] text-[var(--color-text)]'
            : 'text-[var(--color-text-dim)] hover:text-[var(--color-text)]'
        }`}
      >
        {t('toolbar.collapseGaps')}
      </button>
      <button
        type="button"
        role="switch"
        aria-checked={normalizeBars}
        onClick={() => onNormalizeBars(!normalizeBars)}
        title={t('toolbar.logDurationTitle')}
        className={`rounded px-1.5 py-0.5 ${
          normalizeBars
            ? 'bg-[var(--color-accent-soft)] text-[var(--color-text)]'
            : 'text-[var(--color-text-dim)] hover:text-[var(--color-text)]'
        }`}
      >
        {t('toolbar.logDuration')}
      </button>
    </>
  )
}
