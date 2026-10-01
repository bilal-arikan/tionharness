// One decision authority: its mode (off / shadow / on), threshold, which model
// answers for it (with an optional fallback and challenger) and how it has
// been doing, with a hint when the numbers support switching it on or letting
// the challenger take over.
import { useTranslation } from 'react-i18next'
import type {
  DeciderAuthority,
  DeciderAuthorityStats,
  DeciderConfig,
  DeciderModelInstance,
} from '@/types/decider'
import { Badge } from '@/shared/components'
import { OptionPills } from '@/shared/components/OptionPills'
import { inputCls } from '@/features/settings/primitives'
import { percent, usd } from '@/shared/lib/format'
import {
  adviceFor,
  agreement,
  authorityConfig,
  challengerAdvice,
  challengerAgreement,
  modelLabel,
  withAuthority,
  type Advice,
} from './deciderModel'

interface Props {
  authority: DeciderAuthority
  draft: DeciderConfig
  models: DeciderModelInstance[]
  defaultId: string
  defaultLabel: string
  stats?: DeciderAuthorityStats
  onChange: (next: DeciderConfig) => void
}

const ADVICE_TONE: Record<Advice['kind'], string> = {
  ready: 'text-[var(--color-success)]',
  keep: 'text-[var(--color-warning)]',
  collect: 'text-[var(--color-text-dim)]',
}

export function DeciderAuthorityRow({
  authority,
  draft,
  models,
  defaultId,
  defaultLabel,
  stats,
  onChange,
}: Props) {
  const { t } = useTranslation('decider')
  const ac = authorityConfig(draft, authority)
  const rate = agreement(stats)
  const advice = adviceFor(ac.mode, stats)
  const challengerRate = challengerAgreement(stats)
  const cAdvice = challengerAdvice(ac.challenger, stats)
  const workflow = ['session', 'context', 'collaboration'].includes(authority.group)
  const label = t(`authority.${authority.id}.label`, { defaultValue: authority.label })
  const description = t(`authority.${authority.id}.description`, {
    defaultValue: authority.description,
  })
  const thresholdHint = t(`authority.${authority.id}.threshold`, {
    defaultValue: authority.thresholdHint,
  })
  const set = (patch: Parameters<typeof withAuthority>[2]) =>
    onChange(withAuthority(draft, authority, patch))
  // A fallback or challenger equal to the model that already answers would
  // never be asked, so it is not offered.
  const answering = ac.model || defaultId
  const others = models.filter((m) => m.id !== answering)

  const adviceText = (a: Advice, challenger: boolean) => {
    if (a.kind === 'collect') {
      return challenger
        ? t('stats.challengerCollect', { done: a.done, needed: a.needed })
        : t('stats.needsData', { done: a.done, needed: a.needed })
    }
    if (a.kind === 'ready')
      return challenger ? t('stats.challengerReady') : t('stats.readyToSwitch')
    return challenger ? t('stats.challengerKeep') : t('stats.keepShadow')
  }

  return (
    <div
      className="space-y-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] p-3"
      data-testid={`decider-authority-${authority.id}`}
    >
      <div className="space-y-1">
        <p className="flex flex-wrap items-center gap-1.5 text-sm font-medium">
          {label}
          <Badge tone="accent">
            {t(`pattern.${authority.pattern}`, { defaultValue: authority.pattern })}
          </Badge>
          {authority.explicit && <Badge tone="muted">{t('authorities.explicit')}</Badge>}
          {authority.failClosed && (
            <span title={t('authorities.failClosedHint')}>
              <Badge tone="warning">{t('authorities.failClosed')}</Badge>
            </span>
          )}
        </p>
        <p className="text-xs text-[var(--color-text-dim)]">{description}</p>
      </div>
      <OptionPills
        value={ac.mode}
        onChange={(v) => set({ mode: v as DeciderAuthority['defaultMode'] })}
        options={authority.modes.map((m) => ({
          value: m,
          label: t(`modeLabel.${m}`),
          hint: t(`modeHint.${m}`),
        }))}
        ariaLabel={label}
        testid={`decider-mode-${authority.id}`}
      />
      <label className="flex items-center gap-3 text-xs text-[var(--color-text-dim)]">
        <span className="shrink-0 font-medium text-[var(--color-text)]">{t('threshold')}</span>
        <input
          type="range"
          min={0.5}
          max={0.99}
          step={0.01}
          value={ac.threshold}
          disabled={ac.mode === 'off' || !draft.enabled}
          onChange={(e) => set({ threshold: Number(e.target.value) })}
          className="min-w-0 flex-1 accent-[var(--color-accent)]"
          aria-label={`${label} — ${t('threshold')}`}
        />
        <span className="w-10 shrink-0 text-end tabular-nums text-[var(--color-text)]">
          {percent(ac.threshold)}
        </span>
      </label>
      <p className="text-xs text-[var(--color-text-dim)]">{thresholdHint}</p>

      {workflow && (
        <details className="rounded-md border border-[var(--color-border)] p-2">
          <summary className="min-h-11 cursor-pointer content-center text-xs font-medium">
            {t('workflow.limits', { defaultValue: 'Workflow limits' })}
          </summary>
          <div className="grid gap-3 py-2 sm:grid-cols-2">
            {(
              [
                {
                  key: 'candidateLimit',
                  value: ac.candidateLimit ?? 32,
                  min: 4,
                  max: 48,
                  show: authority.id !== 'clarification',
                },
                {
                  key: 'selectionLimit',
                  value: ac.selectionLimit ?? 8,
                  min: 1,
                  max: 16,
                  show: ['session-setup', 'context-reminder', 'worker-review'].includes(
                    authority.id,
                  ),
                },
                {
                  key: 'remindEvery',
                  value: ac.remindEvery ?? 3,
                  min: 1,
                  max: 20,
                  show: authority.id === 'context-reminder',
                },
                {
                  key: 'contextBudget',
                  value: ac.contextBudget ?? 8192,
                  min: 1024,
                  max: 32768,
                  show: ['session-setup', 'compact-retention', 'context-reminder'].includes(
                    authority.id,
                  ),
                },
              ] as const
            )
              .filter((field) => field.show)
              .map((field) => (
                <label key={field.key} className="flex min-w-0 flex-col gap-1 text-xs">
                  {t(`workflow.${field.key}`, { defaultValue: field.key })}
                  <input
                    type="number"
                    className={inputCls}
                    min={field.min}
                    max={field.max}
                    value={field.value}
                    disabled={!draft.enabled || ac.mode === 'off'}
                    onChange={(event) => {
                      const value = Number(event.target.value)
                      if (Number.isFinite(value))
                        set({
                          [field.key]: Math.round(Math.min(field.max, Math.max(field.min, value))),
                        })
                    }}
                  />
                </label>
              ))}
          </div>
          <p className="text-xs text-[var(--color-text-dim)]">
            {t('workflow.measureHint', {
              defaultValue:
                'Agreement measures behavior differences. Use session feedback and applied outcomes to assess usefulness.',
            })}
          </p>
        </details>
      )}

      <div className="grid gap-2 sm:grid-cols-3">
        <label className="flex flex-col gap-1 text-xs">
          <span className="font-medium">{t('authorities.model')}</span>
          <select
            className={inputCls}
            value={ac.model}
            onChange={(e) => set({ model: e.target.value })}
            data-testid={`decider-model-select-${authority.id}`}
          >
            <option value="">
              {t('authorities.modelDefault', { label: defaultLabel || '—' })}
            </option>
            {models.map((m) => (
              <option key={m.id} value={m.id}>
                {m.label || m.id}
              </option>
            ))}
          </select>
        </label>
        <label className="flex flex-col gap-1 text-xs">
          <span className="font-medium">{t('authorities.fallback')}</span>
          <select
            className={inputCls}
            value={ac.fallback}
            onChange={(e) => set({ fallback: e.target.value })}
            data-testid={`decider-fallback-select-${authority.id}`}
          >
            <option value="">{t('authorities.none')}</option>
            {others.map((m) => (
              <option key={m.id} value={m.id}>
                {m.label || m.id}
              </option>
            ))}
          </select>
        </label>
        <label className="flex flex-col gap-1 text-xs">
          <span className="font-medium">{t('authorities.challenger')}</span>
          <select
            className={inputCls}
            value={ac.challenger}
            onChange={(e) => set({ challenger: e.target.value })}
            data-testid={`decider-challenger-select-${authority.id}`}
          >
            <option value="">{t('authorities.none')}</option>
            {others.map((m) => (
              <option key={m.id} value={m.id}>
                {m.label || m.id}
              </option>
            ))}
          </select>
        </label>
      </div>

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
          {stats.fallbacks > 0 && (
            <span>
              {t('stats.fallbacks')}:{' '}
              <span className="tabular-nums text-[var(--color-text)]">{stats.fallbacks}</span>
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
      {advice && !workflow && (
        <p className={`text-xs ${ADVICE_TONE[advice.kind]}`}>{adviceText(advice, false)}</p>
      )}

      {ac.challenger && (
        <div
          className="space-y-0.5 text-xs text-[var(--color-text-dim)]"
          data-testid={`decider-challenger-${authority.id}`}
        >
          <p>
            <span className="font-medium text-[var(--color-text)]">
              {t('stats.challenger', { label: modelLabel(models, ac.challenger) })}
            </span>
            {': '}
            {stats && stats.challengerCompared > 0 && challengerRate !== null
              ? t('stats.challengerLine', {
                  rate: percent(challengerRate),
                  done: stats.challengerCompared,
                  p50: stats.challengerP50Ms,
                  cost: usd(stats.challengerCostUsd),
                })
              : t('stats.challengerNone')}
          </p>
          {cAdvice && <p className={ADVICE_TONE[cAdvice.kind]}>{adviceText(cAdvice, true)}</p>}
        </div>
      )}
    </div>
  )
}
