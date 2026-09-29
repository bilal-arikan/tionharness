// Shared badge components for the Insight cockpit.
import { useTranslation } from 'react-i18next'

export function ChannelBadge({ channel }: { channel: string }) {
  const { t } = useTranslation('insight')
  const appFix = channel === 'app-fix'
  const recipe = channel === 'recipe-opt'
  return (
    <span
      className={`rounded px-1.5 py-0.5 text-xs ${
        appFix
          ? 'bg-[var(--color-danger)]/15 text-[var(--color-danger)]'
          : recipe
            ? 'bg-[#6B7FD8]/15 text-[#6B7FD8]'
            : 'bg-[var(--color-accent)]/15 text-[var(--color-accent)]'
      }`}
      title={recipe ? t('badges.recipeOptimizerTitle') : undefined}
    >
      {appFix ? 'app-fix' : recipe ? '✦ recipe-opt' : 'workspace-opt'}
    </span>
  )
}

export function SeverityBadge({ severity }: { severity: string }) {
  const { t } = useTranslation('insight')
  const color =
    severity === 'high'
      ? 'var(--color-danger)'
      : severity === 'med' || severity === 'medium'
        ? 'var(--color-warning)'
        : 'var(--color-text-dim)'
  return (
    <span
      className="rounded px-1.5 py-0.5 text-xs font-medium"
      style={{ color, backgroundColor: `color-mix(in srgb, ${color} 14%, transparent)` }}
    >
      {severity === 'high'
        ? t('severity.high')
        : severity === 'med' || severity === 'medium'
          ? t('severity.medium')
          : severity === 'low'
            ? t('severity.low')
            : severity}
    </span>
  )
}

export function RegressedBadge() {
  const { t } = useTranslation('insight')
  return (
    <span className="rounded bg-[var(--color-danger)]/15 px-1.5 py-0.5 text-xs font-semibold text-[var(--color-danger)]">
      ⚠ {t('badges.regression')}
    </span>
  )
}

export function StatusBadge({ status }: { status: string }) {
  const { t } = useTranslation('insight')
  return (
    <span className="rounded border border-[var(--color-border)] px-1.5 py-0.5 text-xs text-[var(--color-text-dim)]">
      {t(`status.${status}`, { defaultValue: status })}
    </span>
  )
}
