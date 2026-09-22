// One decision provider as a card, laid out like a provider instance card
// (settings/providers/ProviderInstanceList): label, kind, state badges, id and
// model, where it answers from, who relies on it — and test / edit / delete.
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Scale, Trash2 } from 'lucide-react'
import type { DeciderBackend, DeciderModelInstance, DeciderTestResult } from '@/types/decider'
import { formatTime } from '@/shared/lib/intl'
import { percent, usd } from '@/shared/lib/format'
import { modelTone } from './deciderModel'
import { isLocalUrl } from './modelDraft'
import { cardCls, deleteButtonCls, pillCls, smallButtonCls } from './providerStyles'

interface Props {
  model: DeciderModelInstance
  backend?: DeciderBackend
  authorityLabel: (id: string) => string
  onEdit: () => void
  onDelete: () => void
  onTest: () => Promise<DeciderTestResult>
}

const TONE_TEXT = {
  success: 'text-[var(--color-success)]',
  warning: 'text-[var(--color-warning)]',
  danger: 'text-[var(--color-warning)]',
  muted: 'text-[var(--color-text-dim)]',
} as const

export function DeciderProviderCard({
  model,
  backend,
  authorityLabel,
  onEdit,
  onDelete,
  onTest,
}: Props) {
  const { t } = useTranslation('decider')
  const [testing, setTesting] = useState(false)
  const [test, setTest] = useState<DeciderTestResult | null>(null)
  const tone = modelTone(model)
  const state = !model.enabled
    ? t('providers.state.disabled')
    : model.status.ready
      ? t('providers.state.ready')
      : model.status.backoffUntil
        ? t('providers.state.paused')
        : t('providers.state.problem')
  const usedBy = model.usedBy.map((u) =>
    u === 'default' ? t('providers.usedByDefault') : authorityLabel(u),
  )
  const local = !!model.status.endpoint && isLocalUrl(`http://${model.status.endpoint}`)
  const where =
    model.credentials === 'provider'
      ? model.status.provider && t('providers.viaProvider', { id: model.status.provider })
      : model.status.endpoint

  const runTest = async () => {
    setTesting(true)
    setTest(null)
    try {
      setTest(await onTest())
    } finally {
      setTesting(false)
    }
  }

  const answers = test?.response?.answers
  const approval = answers?.needs_approval?.probability
  const risk = answers?.risk?.score

  return (
    <div className={cardCls} data-testid={`decider-provider-${model.id}`}>
      <div className="flex items-center justify-between gap-2">
        <div className="flex min-w-0 items-center gap-2">
          <Scale size={15} className="shrink-0 text-[var(--color-accent)]" />
          <span className="truncate text-sm font-medium">{model.label || model.id}</span>
          <span className="shrink-0 text-[10px] uppercase tracking-wide text-[var(--color-text-dim)]">
            {t(`backend.${model.backend}.label`, { defaultValue: backend?.label ?? model.backend })}
          </span>
          {!model.enabled && (
            <span className={`${pillCls} text-[var(--color-text-dim)]`}>
              {t('providers.state.disabled')}
            </span>
          )}
          {local && (
            <span className={`${pillCls} text-[var(--color-success)]`}>{t('providers.free')}</span>
          )}
          {backend && !backend.calibrated && (
            <span className={`${pillCls} text-[var(--color-text-dim)]`}>
              {t('providers.approximate')}
            </span>
          )}
        </div>
        {model.enabled && (
          <span className={`${pillCls} font-medium ${TONE_TEXT[tone]}`}>
            {tone === 'success' ? '✓ ' : '⚠ '}
            {state}
          </span>
        )}
      </div>

      <div className="grid gap-x-3 gap-y-1 text-xs sm:grid-cols-2">
        <div className="min-w-0 truncate" title={model.id}>
          <span className="text-[var(--color-text-dim)]">{t('providers.id')}: </span>
          {model.id}
        </div>
        <div className="min-w-0 truncate" title={model.model}>
          <span className="text-[var(--color-text-dim)]">{t('providers.model')}: </span>
          {model.model}
        </div>
        {where && (
          <div className="min-w-0 truncate" title={where}>
            <span className="text-[var(--color-text-dim)]">{t('providers.endpoint')}: </span>
            {where}
          </div>
        )}
        {usedBy.length > 0 && (
          <div className="min-w-0 truncate" title={usedBy.join(', ')}>
            <span className="text-[var(--color-text-dim)]">{t('providers.usedBy')}: </span>
            {usedBy.join(', ')}
          </div>
        )}
      </div>

      {model.stats && model.stats.calls > 0 && (
        <p className="text-[11px] tabular-nums text-[var(--color-text-dim)]">
          {t('providers.statsLine', {
            calls: model.stats.calls,
            p50: model.stats.p50Ms,
            cost: usd(model.stats.costUsd),
          })}
        </p>
      )}
      {model.enabled && model.status.problem && (
        <p className="break-words text-[11px] text-[var(--color-danger)]">{model.status.problem}</p>
      )}
      {model.status.backoffUntil ? (
        <p className="text-[11px] text-[var(--color-warning)]">
          {t('status.pausedUntil', { time: formatTime(model.status.backoffUntil) })}
        </p>
      ) : null}
      {test && (
        <div className="space-y-0.5 text-[11px]" data-testid={`decider-provider-test-${model.id}`}>
          <p
            className={`break-words ${test.ok ? 'text-[var(--color-success)]' : 'text-[var(--color-danger)]'}`}
          >
            {test.ok && test.response
              ? t('providers.testOk', {
                  ms: test.response.latencyMs,
                  cost: usd(test.response.usage.costUsd),
                  probability: approval === undefined ? '?' : percent(approval),
                  level: risk === undefined ? '?' : risk.toFixed(1),
                })
              : t('providers.testFailed', { error: test.error ?? '' })}
          </p>
          {test.response?.warnings?.map((w) => (
            <p key={w} className="text-[var(--color-warning)]">
              {w}
            </p>
          ))}
        </div>
      )}

      <div className="flex items-center justify-end gap-1.5">
        <button
          type="button"
          onClick={runTest}
          disabled={testing}
          className={smallButtonCls}
          data-testid="decider-provider-test"
        >
          {testing ? t('providers.testing') : t('providers.test')}
        </button>
        <button
          type="button"
          onClick={onEdit}
          className={smallButtonCls}
          data-testid="decider-provider-edit"
        >
          {t('providers.edit')}
        </button>
        <button
          type="button"
          onClick={onDelete}
          className={deleteButtonCls}
          aria-label={t('providers.delete')}
          title={t('providers.delete')}
          data-testid="decider-provider-delete"
        >
          <Trash2 size={13} />
        </button>
      </div>
    </div>
  )
}
