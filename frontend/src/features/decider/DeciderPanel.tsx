// Decision authorities screen (backend: internal/decider): the master switch,
// the default decision provider, every authority's mode, threshold, provider,
// fallback and challenger, and the recent decisions. The decision providers
// themselves are managed on Settings → Providers (DeciderProviders). Self
// managed: it loads and saves /api/decider itself, outside the app-settings
// draft, because the decider keeps its own settings documents.
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Scale } from 'lucide-react'
import { api } from '@/api'
import type { DeciderConfig, DeciderTestResult, DeciderView } from '@/types/decider'
import { Badge, Button, InfoPopover, LoadingState, toast } from '@/shared/components'
import { Field, Toggle, inputCls } from '@/features/settings/primitives'
import { formatTime } from '@/shared/lib/intl'
import { percent, usd } from '@/shared/lib/format'
import {
  defaultModelId,
  modelLabel,
  pruneModelRefs,
  sameConfig,
  setupSteps,
  statusTone,
} from './deciderModel'
import { DeciderSetupGuide } from './DeciderSetupGuide'
import { DeciderAuthorities } from './DeciderAuthorities'
import { DeciderActivity } from './DeciderActivity'
import { DeciderDebug } from './DeciderDebug'

interface Props {
  onError: (msg: string) => void
  // onOpenProviders switches to Settings → Providers, where decision providers
  // are added and edited.
  onOpenProviders?: () => void
}

export function DeciderPanel({ onError, onOpenProviders }: Props) {
  const { t } = useTranslation('decider')
  const [view, setView] = useState<DeciderView | null>(null)
  const [draft, setDraft] = useState<DeciderConfig | null>(null)
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const [test, setTest] = useState<DeciderTestResult | null>(null)
  // saved is the configuration the server last reported, read by applyView
  // after an await (a render's closure may be stale by then).
  const saved = useRef<DeciderConfig | null>(null)

  // applyView takes a view the server sent. A draft with unsaved edits survives
  // it (minus references to a model that is gone); a clean one follows the
  // server.
  const applyView = (v: DeciderView) => {
    const before = saved.current
    saved.current = v.config
    setView(v)
    setDraft((d) =>
      d && before && !sameConfig(d, before)
        ? pruneModelRefs(
            d,
            v.models.map((m) => m.id),
          )
        : v.config,
    )
  }

  // Load once on mount. onError may be a fresh function on every parent render,
  // and a reload would overwrite unsaved edits in the draft.
  useEffect(() => {
    api
      .getDecider()
      .then(applyView)
      .catch((e) => onError(t('actions.loadFailed', { error: (e as Error).message })))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  if (!view || !draft) return <LoadingState label={t('title')} />

  const dirty = !sameConfig(draft, view.config)
  const tone = statusTone(view.status, view.config.enabled)
  const defaultLabel = modelLabel(
    view.models,
    view.status.model || defaultModelId(view.config, view.models),
  )
  // The model "automatic" resolves to: the first enabled one.
  const autoModel = modelLabel(
    view.models,
    defaultModelId({ ...draft, defaultModel: '' }, view.models),
  )

  const save = async () => {
    setSaving(true)
    try {
      const v = await api.saveDecider(draft)
      saved.current = v.config
      setView(v)
      setDraft(v.config)
      toast.success(t('actions.saved'))
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const runTest = async () => {
    setTesting(true)
    setTest(null)
    try {
      setTest(await api.testDecider())
      // The test refreshes the health badge (a failure may have paused the model).
      applyView(await api.getDecider())
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setTesting(false)
    }
  }

  const approval = test?.response?.answers?.needs_approval?.probability

  return (
    <div className="space-y-4" data-testid="decider-panel">
      <div className="space-y-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-4 text-sm text-[var(--color-text-dim)]">
        <p className="flex items-center gap-2 font-medium text-[var(--color-text)]">
          <Scale size={15} /> {t('title')}
          <InfoPopover text={t('intro')} />
          <Badge tone={tone}>
            {!view.config.enabled
              ? t('status.off')
              : view.status.ready
                ? t('status.ready')
                : t('status.notReady')}
          </Badge>
        </p>
        <p className="text-xs">{t('privacy')}</p>
        {defaultLabel && <p className="text-xs">{t('status.model', { label: defaultLabel })}</p>}
        {view.status.backoffUntil ? (
          <p className="text-xs text-[var(--color-warning)]">
            {t('status.pausedUntil', { time: formatTime(view.status.backoffUntil) })}
          </p>
        ) : null}
        {view.status.problem && view.config.enabled && (
          <p className="break-words text-xs text-[var(--color-danger)]">{view.status.problem}</p>
        )}
      </div>

      <DeciderSetupGuide
        steps={setupSteps(view)}
        actions={
          onOpenProviders && {
            account: (
              <button
                type="button"
                onClick={onOpenProviders}
                className="text-[var(--color-accent)] underline-offset-2 hover:underline"
              >
                {t('setup.goProviders')}
              </button>
            ),
            provider: (
              <button
                type="button"
                onClick={onOpenProviders}
                className="text-[var(--color-accent)] underline-offset-2 hover:underline"
              >
                {t('setup.goProviders')}
              </button>
            ),
          }
        }
      />

      <Toggle
        label={t('enabled')}
        hint={t('enabledHint')}
        checked={draft.enabled}
        onChange={(v) => setDraft({ ...draft, enabled: v })}
      />

      <p className="flex flex-wrap items-center gap-x-2 text-xs text-[var(--color-text-dim)]">
        {view.models.length === 0 ? (
          <span className="text-[var(--color-warning)]">{t('noModels')}</span>
        ) : (
          <span>{t('providersSummary', { n: view.models.length })}</span>
        )}
        {onOpenProviders && (
          <button
            type="button"
            onClick={onOpenProviders}
            className="text-[var(--color-accent)] underline-offset-2 hover:underline"
            data-testid="decider-open-providers"
          >
            {t('openProviders')}
          </button>
        )}
      </p>

      {view.models.length > 0 && (
        <Field label={t('defaultModel')} hint={t('defaultModelHint')}>
          <select
            className={inputCls}
            value={draft.defaultModel}
            onChange={(e) => setDraft({ ...draft, defaultModel: e.target.value })}
            data-testid="decider-default-model"
          >
            <option value="">
              {autoModel ? `${t('defaultModelAuto')} (${autoModel})` : t('defaultModelAuto')}
            </option>
            {view.models.map((m) => (
              <option key={m.id} value={m.id}>
                {m.label || m.id}
              </option>
            ))}
          </select>
        </Field>
      )}

      <DeciderAuthorities view={view} draft={draft} onChange={setDraft} />

      <div className="flex flex-wrap items-center gap-2">
        <Button onClick={save} disabled={!dirty || saving} data-testid="decider-save">
          {saving ? t('actions.saving') : t('actions.save')}
        </Button>
        <Button
          variant="secondary"
          onClick={runTest}
          disabled={testing || dirty || view.models.length === 0}
          data-testid="decider-test"
        >
          {testing ? t('actions.testing') : t('actions.test')}
        </Button>
        {dirty && (
          <span className="text-xs text-[var(--color-warning)]">{t('actions.unsaved')}</span>
        )}
      </div>
      {test && (
        <p
          className={`break-words text-xs ${test.ok ? 'text-[var(--color-success)]' : 'text-[var(--color-danger)]'}`}
          data-testid="decider-test-result"
        >
          {test.ok && test.response
            ? t('providers.testOk', {
                ms: test.response.latencyMs,
                cost: usd(test.response.usage.costUsd),
                probability: approval === undefined ? '?' : percent(approval),
                level: test.response.answers?.risk?.score?.toFixed(1) ?? '?',
              })
            : t('providers.testFailed', { error: test.error ?? '' })}
        </p>
      )}

      <DeciderDebug view={view} />
      <DeciderActivity
        records={view.recent}
        days={view.statsDays}
        models={view.models}
        authorities={view.authorities}
      />
    </div>
  )
}
