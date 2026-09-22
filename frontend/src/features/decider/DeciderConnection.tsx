// Connection settings of the decision model: backend, the provider account whose
// key it borrows, the model and the per-attempt timeout.
import { useTranslation } from 'react-i18next'
import type { DeciderBackend, DeciderConfig, DeciderInstance } from '@/types/decider'
import { Field, NumberField, inputCls } from '@/features/settings/primitives'
import { SectionHead } from '@/shared/components'
import { isCustomModel, modelPrice } from './deciderModel'

interface Props {
  draft: DeciderConfig
  backends: DeciderBackend[]
  candidates: DeciderInstance[]
  onChange: (next: DeciderConfig) => void
}

const CUSTOM = '__custom__'

export function DeciderConnection({ draft, backends, candidates, onChange }: Props) {
  const { t } = useTranslation('decider')
  const backend = backends.find((b) => b.id === draft.backend)
  const custom = isCustomModel(backend, draft.model)
  const price = modelPrice(backend, draft.model)
  // A configured instance that is no longer usable still shows, marked, so the
  // select never silently displays a different account than the one saved.
  const staleInstance =
    draft.providerInstanceId !== '' && !candidates.some((c) => c.id === draft.providerInstanceId)
  const set = <K extends keyof DeciderConfig>(key: K, value: DeciderConfig[K]) =>
    onChange({ ...draft, [key]: value })

  return (
    <section className="space-y-3 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
      <SectionHead>{t('general')}</SectionHead>
      {backends.length > 1 && (
        <Field label={t('backend')} hint={backend?.description}>
          <select
            className={inputCls}
            value={draft.backend}
            onChange={(e) => {
              const next = backends.find((b) => b.id === e.target.value)
              onChange({ ...draft, backend: e.target.value, model: next?.defaultModel ?? '' })
            }}
          >
            {backends.map((b) => (
              <option key={b.id} value={b.id}>
                {b.label}
              </option>
            ))}
          </select>
        </Field>
      )}
      <Field label={t('instance')} hint={t('instanceHint')}>
        <select
          className={inputCls}
          value={draft.providerInstanceId}
          onChange={(e) => set('providerInstanceId', e.target.value)}
          data-testid="decider-instance"
        >
          <option value="">{t('instanceAuto')}</option>
          {candidates.map((c) => (
            <option key={c.id} value={c.id}>
              {c.label ? `${c.label} (${c.id})` : c.id}
            </option>
          ))}
          {staleInstance && (
            <option value={draft.providerInstanceId}>
              {t('instanceUnavailable', { id: draft.providerInstanceId })}
            </option>
          )}
        </select>
      </Field>
      {candidates.length === 0 && (
        <p className="text-xs text-[var(--color-warning)]">{t('instanceMissing')}</p>
      )}
      <Field label={t('model')} hint={t('modelHint')}>
        <select
          className={inputCls}
          value={custom ? CUSTOM : draft.model}
          onChange={(e) => set('model', e.target.value === CUSTOM ? '' : e.target.value)}
          data-testid="decider-model"
        >
          {(backend?.models ?? []).map((m) => (
            <option key={m.id} value={m.id}>
              {m.label} — {m.id}
            </option>
          ))}
          <option value={CUSTOM}>{t('modelCustom')}</option>
        </select>
      </Field>
      {(custom || draft.model === '') && (
        <input
          className={inputCls}
          value={draft.model}
          placeholder={t('modelCustomPlaceholder')}
          onChange={(e) => set('model', e.target.value.trim())}
          aria-label={t('modelCustom')}
        />
      )}
      {price !== null && (
        <p className="text-xs text-[var(--color-text-dim)]">{t('price', { input: price })}</p>
      )}
      <NumberField
        label={t('timeout')}
        hint={t('timeoutHint')}
        value={draft.timeoutMs}
        min={500}
        max={15000}
        step={100}
        onChange={(v) => set('timeoutMs', v)}
      />
    </section>
  )
}
