// One decision point: its mode (off / shadow / on), threshold and how it has been
// doing, with a hint when shadow numbers support switching it on.
import { useTranslation } from 'react-i18next'
import type { DeciderConfig, DeciderSite, DeciderSiteStats } from '@/types/decider'
import { OptionPills } from '@/shared/components/OptionPills'
import { percent, usd } from '@/shared/lib/format'
import { adviceFor, agreement, siteConfig, withSite } from './deciderModel'

interface Props {
  site: DeciderSite
  draft: DeciderConfig
  stats?: DeciderSiteStats
  onChange: (next: DeciderConfig) => void
}

export function DeciderSiteRow({ site, draft, stats, onChange }: Props) {
  const { t } = useTranslation('decider')
  const sc = siteConfig(draft, site)
  const rate = agreement(stats)
  const advice = adviceFor(sc.mode, stats)
  const label = t(`site.${site.id}.label`, { defaultValue: site.label })
  const description = t(`site.${site.id}.description`, { defaultValue: site.description })
  const thresholdHint = t(`site.${site.id}.threshold`, { defaultValue: site.thresholdHint })

  return (
    <div
      className="space-y-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] p-3"
      data-testid={`decider-site-${site.id}`}
    >
      <div>
        <p className="text-sm font-medium">{label}</p>
        <p className="text-xs text-[var(--color-text-dim)]">{description}</p>
      </div>
      <OptionPills
        value={sc.mode}
        onChange={(v) => onChange(withSite(draft, site, { mode: v as DeciderSite['defaultMode'] }))}
        options={site.modes.map((m) => ({
          value: m,
          label: t(`modeLabel.${m}`),
          hint: t(`modeHint.${m}`),
        }))}
        ariaLabel={label}
        testid={`decider-mode-${site.id}`}
      />
      <label className="flex items-center gap-3 text-xs text-[var(--color-text-dim)]">
        <span className="shrink-0 font-medium text-[var(--color-text)]">{t('threshold')}</span>
        <input
          type="range"
          min={0.5}
          max={0.99}
          step={0.01}
          value={sc.threshold}
          disabled={sc.mode === 'off' || !draft.enabled}
          onChange={(e) => onChange(withSite(draft, site, { threshold: Number(e.target.value) }))}
          className="min-w-0 flex-1 accent-[var(--color-accent)]"
          aria-label={`${label} — ${t('threshold')}`}
        />
        <span className="w-10 shrink-0 text-end tabular-nums text-[var(--color-text)]">
          {percent(sc.threshold)}
        </span>
      </label>
      <p className="text-xs text-[var(--color-text-dim)]">{thresholdHint}</p>
      {stats && stats.calls > 0 && (
        <p className="flex flex-wrap gap-x-3 gap-y-1 text-xs text-[var(--color-text-dim)]">
          <span>
            {t('stats.calls')}:{' '}
            <span className="tabular-nums text-[var(--color-text)]">{stats.calls}</span>
          </span>
          {rate !== null && (
            <span title={t('stats.agreementHint')}>
              {t('stats.agreement')}:{' '}
              <span className="tabular-nums text-[var(--color-text)]">{percent(rate)}</span>
            </span>
          )}
          {stats.applied > 0 && (
            <span>
              {t('stats.applied')}:{' '}
              <span className="tabular-nums text-[var(--color-text)]">{stats.applied}</span>
            </span>
          )}
          {stats.errors > 0 && (
            <span className="text-[var(--color-warning)]">
              {t('stats.errors')}: <span className="tabular-nums">{stats.errors}</span>
            </span>
          )}
          <span>
            {t('stats.latency')}:{' '}
            <span className="tabular-nums text-[var(--color-text)]">
              {t('stats.latencyValue', { p50: stats.p50Ms, p95: stats.p95Ms })}
            </span>
          </span>
          <span>
            {t('stats.cost')}:{' '}
            <span className="tabular-nums text-[var(--color-text)]">{usd(stats.costUsd)}</span>
          </span>
        </p>
      )}
      {advice && (
        <p
          className={`text-xs ${
            advice.kind === 'ready'
              ? 'text-[var(--color-success)]'
              : advice.kind === 'keep'
                ? 'text-[var(--color-warning)]'
                : 'text-[var(--color-text-dim)]'
          }`}
        >
          {advice.kind === 'collect'
            ? t('stats.needsData', { done: advice.done, needed: advice.needed })
            : advice.kind === 'ready'
              ? t('stats.readyToSwitch')
              : t('stats.keepShadow')}
        </p>
      )}
    </div>
  )
}
