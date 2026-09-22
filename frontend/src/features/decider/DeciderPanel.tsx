// Settings page of the decision-model layer (backend: internal/decider). Self
// managed: it loads and saves /api/decider itself, outside the app-settings
// draft, because the decider keeps its own settings document.
import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Scale } from 'lucide-react'
import { api } from '@/api'
import type { DeciderConfig, DeciderTestResult, DeciderView } from '@/types/decider'
import { Badge, Button, LoadingState, SectionHead, toast } from '@/shared/components'
import { Toggle } from '@/features/settings/primitives'
import { formatTime } from '@/shared/lib/intl'
import { percent, usd } from '@/shared/lib/format'
import { sameConfig, statsFor, statusTone } from './deciderModel'
import { DeciderConnection } from './DeciderConnection'
import { DeciderSiteRow } from './DeciderSiteRow'
import { DeciderActivity } from './DeciderActivity'

interface Props {
  onError: (msg: string) => void
}

export function DeciderPanel({ onError }: Props) {
  const { t } = useTranslation('decider')
  const [view, setView] = useState<DeciderView | null>(null)
  const [draft, setDraft] = useState<DeciderConfig | null>(null)
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const [test, setTest] = useState<DeciderTestResult | null>(null)

  const apply = useCallback((v: DeciderView) => {
    setView(v)
    setDraft(v.config)
  }, [])

  // Load once on mount. onError may be a fresh function on every parent render,
  // and a reload would overwrite unsaved edits in the draft.
  useEffect(() => {
    api
      .getDecider()
      .then(apply)
      .catch((e) => onError(t('actions.loadFailed', { error: (e as Error).message })))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  if (!view || !draft) return <LoadingState label={t('title')} />

  const dirty = !sameConfig(draft, view.config)
  const tone = statusTone(view.status, view.config.enabled)

  const save = async () => {
    setSaving(true)
    try {
      apply(await api.saveDecider(draft))
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
      // The test refreshes the health badge (a failure may have paused the endpoint).
      const fresh = await api.getDecider()
      setView(fresh)
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
          <Badge tone={tone}>
            {!view.config.enabled
              ? t('status.off')
              : view.status.ready
                ? t('status.ready')
                : t('status.notReady')}
          </Badge>
        </p>
        <p>{t('intro')}</p>
        <p className="text-xs">{t('privacy')}</p>
        {view.status.instance && (
          <p className="text-xs">{t('status.account', { id: view.status.instance })}</p>
        )}
        {view.status.backoffUntil ? (
          <p className="text-xs text-[var(--color-warning)]">
            {t('status.pausedUntil', { time: formatTime(view.status.backoffUntil) })}
          </p>
        ) : null}
        {view.status.problem && view.config.enabled && (
          <p className="break-words text-xs text-[var(--color-danger)]">{view.status.problem}</p>
        )}
      </div>

      <Toggle
        label={t('enabled')}
        hint={t('enabledHint')}
        checked={draft.enabled}
        onChange={(v) => setDraft({ ...draft, enabled: v })}
      />

      <DeciderConnection
        draft={draft}
        backends={view.backends}
        candidates={view.candidates}
        onChange={setDraft}
      />

      <section className="space-y-3 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
        <SectionHead>{t('sites')}</SectionHead>
        <p className="text-xs text-[var(--color-text-dim)]">{t('sitesHint')}</p>
        {view.sites.map((site) => (
          <DeciderSiteRow
            key={site.id}
            site={site}
            draft={draft}
            stats={statsFor(view.stats, site.id)}
            onChange={setDraft}
          />
        ))}
      </section>

      <div className="flex flex-wrap items-center gap-2">
        <Button onClick={save} disabled={!dirty || saving} data-testid="decider-save">
          {saving ? t('actions.saving') : t('actions.save')}
        </Button>
        <Button
          variant="secondary"
          onClick={runTest}
          disabled={testing || dirty}
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
            ? t('actions.testOk', {
                ms: test.response.latencyMs,
                cost: usd(test.response.usage.costUsd),
                probability: approval === undefined ? '?' : percent(approval),
              })
            : t('actions.testFailed', { error: test.error ?? '' })}
        </p>
      )}

      <DeciderActivity records={view.recent} days={view.statsDays} />
    </div>
  )
}
