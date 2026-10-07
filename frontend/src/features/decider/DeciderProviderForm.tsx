// The add/edit form of a decision provider, laid out like the provider instance
// form next to it (settings/providers/ProviderInstanceForm): kind (backend,
// locked while editing) and label, the enabled switch, the model, where the
// credentials come from, and every field the backend declares — all from the
// backend's manifest, no per-backend branch here.
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Plus } from 'lucide-react'
import type {
  DeciderBackend,
  DeciderModelInput,
  DeciderModelInstance,
  DeciderPreset,
  DeciderProviderInstance,
} from '@/types/decider'
import { isCustomModel, modelPrice } from './deciderModel'
import {
  API_KEY,
  canBorrow,
  draftFromBackend,
  draftFromPreset,
  isLocalUrl,
  keyMissing,
  toInput,
} from './modelDraft'
import { InfoPopover } from '@/shared/components'
import { DeciderFieldInput, FieldShell } from './DeciderFieldInput'
import { fieldInputCls, formCls, primaryButtonCls, secondaryButtonCls } from './providerStyles'

interface Props {
  backends: DeciderBackend[]
  candidates: Record<string, DeciderProviderInstance[]>
  initial: DeciderModelInput
  editing?: DeciderModelInstance
  onCancel: () => void
  onSave: (input: DeciderModelInput) => Promise<void>
}

const CUSTOM = '__custom__'

// presetKey identifies a preset across backends in the template select.
const presetKey = (b: DeciderBackend, p: DeciderPreset) => `${b.id}/${p.id}`

export function DeciderProviderForm({
  backends,
  candidates,
  initial,
  editing,
  onCancel,
  onSave,
}: Props) {
  const { t } = useTranslation('decider')
  const [draft, setDraft] = useState<DeciderModelInput>(initial)
  const [template, setTemplate] = useState('')
  const [keyText, setKeyText] = useState('')
  const [clearKey, setClearKey] = useState(false)
  const [custom, setCustom] = useState(() =>
    isCustomModel(
      backends.find((b) => b.id === initial.backend),
      initial.model,
    ),
  )
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')

  const backend = backends.find((b) => b.id === draft.backend)
  const kindInfo = backend
    ? t(`backend.${backend.id}.description`, { defaultValue: backend.description ?? '' })
    : ''
  const backendLabel = (b: DeciderBackend) => t(`backend.${b.id}.label`, { defaultValue: b.label })
  const presetLabel = (p: DeciderPreset) => t(`preset.${p.id}.label`, { defaultValue: p.label })
  const keyStored = !!editing?.secretsSet[API_KEY]
  const providers = candidates[draft.backend] ?? []
  const staleProvider =
    draft.providerInstanceId !== '' && !providers.some((p) => p.id === draft.providerInstanceId)
  const price = modelPrice(backend, draft.model)
  const local =
    draft.credentials === 'own' && isLocalUrl(draft.baseUrl || backend?.defaultBaseUrl || '')
  const missingKey = keyMissing(backend, draft, keyText, keyStored, clearKey)
  const set = <K extends keyof DeciderModelInput>(key: K, value: DeciderModelInput[K]) =>
    setDraft((d) => ({ ...d, [key]: value }))

  const applyTemplate = (key: string) => {
    setTemplate(key)
    for (const b of backends) {
      const p = b.presets?.find((x) => presetKey(b, x) === key)
      if (p) {
        setDraft({ ...draftFromPreset(b, p), label: presetLabel(p) })
        setCustom(isCustomModel(b, p.model))
        return
      }
    }
  }

  const save = async () => {
    setBusy(true)
    setErr('')
    try {
      await onSave(toInput(draft, keyText, clearKey))
    } catch (e) {
      setErr((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className={formCls} data-testid="decider-provider-form">
      <div className="text-xs font-medium">
        {editing ? t('form.editTitle', { label: editing.label || editing.id }) : t('form.newTitle')}
      </div>

      {!editing && (
        <select
          className={fieldInputCls}
          value={template}
          onChange={(e) => applyTemplate(e.target.value)}
          aria-label={t('form.template')}
          data-testid="decider-provider-template"
        >
          <option value="">{t('form.templateNone')}</option>
          {backends.flatMap((b) =>
            (b.presets ?? []).map((p) => (
              <option key={presetKey(b, p)} value={presetKey(b, p)}>
                {presetLabel(p)}
              </option>
            )),
          )}
        </select>
      )}

      <div className="grid grid-cols-2 gap-1.5">
        {/* The selected kind's description sits behind the (ⓘ) beside it. */}
        <div className="flex min-w-0 items-center gap-1">
          <select
            className={`${fieldInputCls} min-w-0 flex-1`}
            value={draft.backend}
            disabled={!!editing}
            aria-label={t('form.kind')}
            onChange={(e) => {
              const next = backends.find((b) => b.id === e.target.value)
              if (next) {
                setDraft({ ...draftFromBackend(next), label: backendLabel(next) })
                setCustom(false)
                setTemplate('')
              }
            }}
            data-testid="decider-provider-kind"
          >
            {backends.map((b) => (
              <option key={b.id} value={b.id}>
                {backendLabel(b)}
              </option>
            ))}
          </select>
          {kindInfo && <InfoPopover text={kindInfo} />}
        </div>
        <input
          className={fieldInputCls}
          value={draft.label}
          placeholder={t('form.labelPlaceholder')}
          aria-label={t('form.label')}
          onChange={(e) => set('label', e.target.value)}
          data-testid="decider-provider-label"
        />
      </div>

      <label className="flex items-center gap-1.5 text-xs">
        <input
          type="checkbox"
          checked={draft.enabled}
          onChange={(e) => set('enabled', e.target.checked)}
        />
        {t('form.enabled')}
      </label>

      <div className="grid gap-2 sm:grid-cols-2">
        <FieldShell label={t('form.model')}>
          <select
            className={fieldInputCls}
            value={custom ? CUSTOM : draft.model}
            onChange={(e) => {
              if (e.target.value === CUSTOM) {
                setCustom(true)
                return
              }
              setCustom(false)
              set('model', e.target.value)
            }}
            data-testid="decider-provider-model"
          >
            {(backend?.models ?? []).map((m) => (
              <option key={m.id} value={m.id}>
                {m.label} — {m.id}
              </option>
            ))}
            <option value={CUSTOM}>{t('form.modelCustom')}</option>
          </select>
        </FieldShell>
        {custom && (
          <FieldShell label={t('form.modelCustom')}>
            <input
              className={fieldInputCls}
              value={draft.model}
              placeholder={t('form.modelCustomPlaceholder')}
              onChange={(e) => set('model', e.target.value.trim())}
              data-testid="decider-provider-custom-model"
            />
          </FieldShell>
        )}

        {canBorrow(backend) && (
          <FieldShell label={t('form.credentials')}>
            <select
              className={fieldInputCls}
              value={draft.credentials}
              onChange={(e) =>
                set('credentials', e.target.value as DeciderModelInput['credentials'])
              }
              data-testid="decider-provider-credentials"
            >
              <option value="own">{t('form.credentialsOwn')}</option>
              <option value="provider">{t('form.credentialsProvider')}</option>
            </select>
          </FieldShell>
        )}

        {draft.credentials === 'provider' ? (
          <FieldShell
            label={t('form.provider')}
            help={t('form.credentialsProviderHint')}
            warning={providers.length === 0 ? t('form.providerMissing') : undefined}
          >
            <select
              className={fieldInputCls}
              value={draft.providerInstanceId}
              onChange={(e) => set('providerInstanceId', e.target.value)}
              data-testid="decider-provider-account"
            >
              <option value="">{t('form.providerAuto')}</option>
              {providers.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.label ? `${p.label} (${p.id})` : p.id}
                </option>
              ))}
              {staleProvider && (
                <option value={draft.providerInstanceId}>
                  {t('form.providerUnavailable', { id: draft.providerInstanceId })}
                </option>
              )}
            </select>
          </FieldShell>
        ) : (
          <>
            <FieldShell
              label={t('form.baseUrl')}
              help={t('form.baseUrlHint', { url: backend?.defaultBaseUrl ?? '' })}
            >
              <input
                className={fieldInputCls}
                value={draft.baseUrl}
                placeholder={backend?.defaultBaseUrl}
                onChange={(e) => set('baseUrl', e.target.value.trim())}
                data-testid="decider-provider-baseurl"
              />
            </FieldShell>
            <FieldShell
              label={backend?.keyRequired ? t('form.apiKey') : t('form.apiKeyOptional')}
              warning={missingKey ? t('form.apiKeyRequired') : undefined}
            >
              <input
                type="password"
                autoComplete="off"
                className={fieldInputCls}
                value={keyText}
                placeholder={keyStored && !clearKey ? t('form.apiKeyStored') : ''}
                onChange={(e) => setKeyText(e.target.value)}
                data-testid="decider-provider-key"
              />
              {keyStored && !clearKey && (
                <span className="text-[10px] text-[var(--color-success)]">
                  {t('form.apiKeySaved')}
                </span>
              )}
              {keyStored && (
                <span className="flex items-center gap-1.5 text-[10px] text-[var(--color-text-dim)]">
                  <input
                    type="checkbox"
                    checked={clearKey}
                    onChange={(e) => setClearKey(e.target.checked)}
                  />
                  {t('form.apiKeyClear')}
                </span>
              )}
            </FieldShell>
          </>
        )}

        <FieldShell label={t('form.timeout')} help={t('form.timeoutHint')}>
          <input
            type="number"
            inputMode="numeric"
            min={500}
            max={60000}
            step={100}
            className={fieldInputCls}
            value={draft.timeoutMs || ''}
            onChange={(e) => set('timeoutMs', Number(e.target.value) || 0)}
            data-testid="decider-provider-timeout"
          />
        </FieldShell>
        <FieldShell
          label={t('form.context')}
          help={t('form.contextHint', { tokens: backend?.contextTokens ?? 0 })}
        >
          <input
            type="number"
            inputMode="numeric"
            min={0}
            step={1024}
            className={fieldInputCls}
            value={draft.contextTokens || ''}
            placeholder={String(backend?.contextTokens ?? '')}
            onChange={(e) => set('contextTokens', Number(e.target.value) || 0)}
            data-testid="decider-provider-context"
          />
        </FieldShell>

        {backend?.fields?.map((f) => (
          <DeciderFieldInput
            key={f.key}
            backendId={backend.id}
            field={f}
            value={draft.config[f.key] ?? ''}
            onChange={(v) => setDraft((d) => ({ ...d, config: { ...d.config, [f.key]: v } }))}
          />
        ))}
      </div>

      <div className="space-y-0.5 text-[10px] text-[var(--color-text-dim)]">
        {price !== null && !local && <p>{t('form.price', { input: price })}</p>}
        {local && <p className="text-[var(--color-success)]">{t('form.localFree')}</p>}
        {backend && !backend.calibrated && <p>{t('form.uncalibrated')}</p>}
      </div>

      {err && <div className="break-words text-xs text-[var(--color-danger)]">{err}</div>}

      <div className="flex gap-2">
        <button
          type="button"
          onClick={save}
          disabled={
            busy || !draft.backend || !draft.label.trim() || !draft.model.trim() || missingKey
          }
          className={primaryButtonCls}
          data-testid="decider-provider-save"
        >
          <Plus size={13} /> {busy ? t('form.saving') : editing ? t('form.update') : t('form.add')}
        </button>
        <button
          type="button"
          onClick={onCancel}
          className={secondaryButtonCls}
          data-testid="decider-provider-cancel"
        >
          {t('form.cancel')}
        </button>
      </div>
    </div>
  )
}
