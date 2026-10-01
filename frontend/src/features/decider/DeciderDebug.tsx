import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '@/api'
import type { DeciderDebugReport, DeciderView } from '@/types/decider'
import { Button, LoadingState, SectionHead } from '@/shared/components'
import { inputCls } from '@/features/settings/primitives'
import { percent, usd } from '@/shared/lib/format'
import { formatTime } from '@/shared/lib/intl'
import { groupDebugEvents } from './debugModel'
import { DeciderDebugTrace } from './DeciderDebugTrace'

export function DeciderDebug({ view }: { view: DeciderView }) {
  const { t } = useTranslation('decider')
  const [report, setReport] = useState<DeciderDebugReport | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [authority, setAuthority] = useState('')
  const [instance, setInstance] = useState('')
  const [days, setDays] = useState(7)
  const [refDraft, setRefDraft] = useState('')
  const [ref, setRef] = useState('')
  const [revision, setRevision] = useState(0)
  const [exporting, setExporting] = useState(false)

  useEffect(() => {
    let active = true
    api
      .getDeciderDebug({ days, authority, instance, ref, limit: 500 })
      .then((result) => {
        if (active) {
          setReport(result)
          setError('')
        }
      })
      .catch((e: Error) => {
        if (active) setError(e.message)
      })
      .finally(() => {
        if (active) setLoading(false)
      })
    return () => {
      active = false
    }
  }, [days, authority, instance, ref, revision])

  const reload = () => {
    setLoading(true)
    setRevision((v) => v + 1)
  }
  const download = async () => {
    setExporting(true)
    try {
      const data = await api.getDeciderDebug({ days, authority, instance, ref, limit: 5000 })
      const url = URL.createObjectURL(
        new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' }),
      )
      const link = document.createElement('a')
      link.href = url
      link.download = 'decision-debug.json'
      link.click()
      URL.revokeObjectURL(url)
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setExporting(false)
    }
  }
  const s = report?.summary

  return (
    <section
      className="space-y-3 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
      data-testid="decider-debug"
    >
      <SectionHead>{t('debug.title')}</SectionHead>
      <p className="text-xs text-[var(--color-text-dim)]">{t('debug.privacy')}</p>
      <form
        className="grid gap-2 sm:grid-cols-4"
        onSubmit={(e) => {
          e.preventDefault()
          setRef(refDraft.trim())
          reload()
        }}
      >
        <label className="text-xs">
          {t('debug.authority')}
          <select
            className={inputCls}
            value={authority}
            onChange={(e) => {
              setLoading(true)
              setAuthority(e.target.value)
            }}
          >
            <option value="">{t('debug.all')}</option>
            {view.authorities.map((a) => (
              <option key={a.id} value={a.id}>
                {t(`authority.${a.id}.label`, { defaultValue: a.label })}
              </option>
            ))}
            <option value="model-test">{t('debug.modelTest')}</option>
          </select>
        </label>
        <label className="text-xs">
          {t('authorities.model')}
          <select
            className={inputCls}
            value={instance}
            onChange={(e) => {
              setLoading(true)
              setInstance(e.target.value)
            }}
          >
            <option value="">{t('debug.all')}</option>
            {view.models.map((m) => (
              <option key={m.id} value={m.id}>
                {m.label || m.id}
              </option>
            ))}
          </select>
        </label>
        <label className="text-xs">
          {t('debug.window')}
          <select
            className={inputCls}
            value={days}
            onChange={(e) => {
              setLoading(true)
              setDays(Number(e.target.value))
            }}
          >
            {[1, 7, 30, 90].map((n) => (
              <option key={n} value={n}>
                {t('debug.days', { n })}
              </option>
            ))}
          </select>
        </label>
        <label className="text-xs">
          {t('debug.reference')}
          <input
            className={inputCls}
            value={refDraft}
            maxLength={128}
            onChange={(e) => setRefDraft(e.target.value)}
            placeholder={t('debug.referenceHint')}
          />
        </label>
        <div className="flex flex-wrap gap-2 sm:col-span-4">
          <Button type="submit" variant="secondary" disabled={loading}>
            {t('debug.refresh')}
          </Button>
          <Button
            type="button"
            variant="secondary"
            onClick={download}
            disabled={loading || exporting}
          >
            {t('debug.export')}
          </Button>
        </div>
      </form>
      {loading ? (
        <LoadingState label={t('debug.title')} />
      ) : error ? (
        <p role="alert" className="text-xs text-[var(--color-danger)]">
          {error}
        </p>
      ) : (
        report &&
        s && (
          <>
            <p className="text-xs">
              {t('debug.summary', {
                calls: s.decisions,
                errors: s.errors,
                retries: s.retries,
                fallbacks: s.fallbacks,
                challengers: s.challengers,
                applied: s.applied,
                cost: usd(s.costUsd),
              })}
            </p>
            <p className="text-xs text-[var(--color-text-dim)]">
              {t('stats.latencyValue', { p50: s.p50Ms, p95: s.p95Ms })} ·{' '}
              {t('debug.agreement', {
                n: s.compared,
                rate: s.compared ? percent(s.agreed / s.compared) : '—',
              })}
            </p>
            <p className="text-xs text-[var(--color-text-dim)]">{t('debug.evidenceHint')}</p>
            <p className="text-xs text-[var(--color-text-dim)]">
              {t('debug.testSummary', {
                n: s.tests,
                errors: s.testErrors,
                compared: s.challengerCompared,
                rate: s.challengerCompared
                  ? percent(s.challengerAgreed / s.challengerCompared)
                  : '—',
              })}
            </p>
            <ul className="space-y-1 text-xs text-[var(--color-warning)]">
              {report.issues.map((issue) => (
                <li key={issue.code}>
                  {t(`debug.issue.${issue.code}`, { n: issue.count, defaultValue: issue.code })}
                </li>
              ))}
            </ul>
            <p className="text-xs text-[var(--color-text-dim)]">
              {t('debug.retention', {
                shown: report.events.length,
                matched: report.matchedEvents,
                retained: report.retainedEvents,
                cap: report.capacity,
                oldest: report.oldestAt ? formatTime(report.oldestAt) : '—',
              })}
            </p>
            {report.truncated && (
              <p className="text-xs text-[var(--color-warning)]">{t('debug.truncated')}</p>
            )}
            {report.events.length === 0 ? (
              <p className="text-xs">{t('debug.empty')}</p>
            ) : (
              <div className="space-y-2">
                {groupDebugEvents(report.events).map((trace) => (
                  <DeciderDebugTrace key={trace.id} trace={trace} />
                ))}
              </div>
            )}
          </>
        )
      )}
    </section>
  )
}
