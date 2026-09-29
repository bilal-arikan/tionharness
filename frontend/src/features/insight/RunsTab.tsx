import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { RefreshCw, MessageSquare } from 'lucide-react'
import { api } from '@/api'
import type { InsightRun } from '@/types'
import { relativeTime } from '@/shared/lib/time'
import { buildRoute, parseRoute } from '@/app/url'

// openRunSession deep-links to the run's read-only transcript. The hash carries
// ?kind=insight because insight sessions are hidden from the sidebar's default
// "Tümü" list, so landing there without the chip would show an empty selection.
function openRunSession(sessionId: string) {
  const cur = parseRoute(window.location.hash)
  window.location.hash = buildRoute({
    workspaceId: cur.workspaceId,
    view: 'chat',
    id: sessionId,
    query: { kind: 'insight' },
  })
}

// RunsTab is the scan-run observability log (not sessions): when each scan ran,
// how long it took, and what it covered/produced.
export function RunsTab({ onError }: { onError: (msg: string) => void }) {
  const { t } = useTranslation('insight')
  const [runs, setRuns] = useState<InsightRun[]>([])
  // Loading starts true: the mount fetch below is already in flight on the first
  // paint. run lands results through callbacks only, so the effect can call it;
  // load is the manual-refresh entry point that also re-arms the spinner.
  const [loading, setLoading] = useState(true)

  const run = useCallback(
    () =>
      api
        .getInsightRuns()
        .then(setRuns)
        .catch((e) => onError((e as Error).message))
        .finally(() => setLoading(false)),
    [onError],
  )
  const load = useCallback(() => {
    setLoading(true)
    void run()
  }, [run])

  useEffect(() => {
    void run()
  }, [run])

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <p className="text-sm text-[var(--color-text-dim)]">{t('runs.description')}</p>
        <button
          onClick={load}
          className="flex items-center gap-1 rounded-md px-2 py-1 text-sm hover:bg-[var(--color-surface-2)]"
        >
          <RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} />{' '}
          {t('actions.refresh')}
        </button>
      </div>
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead className="text-left text-xs text-[var(--color-text-dim)]">
            <tr className="border-b border-[var(--color-border)]">
              <th className="py-1 pr-3">{t('runs.time')}</th>
              <th className="py-1 pr-3">{t('runs.duration')}</th>
              <th className="py-1 pr-3">{t('runs.sessions')}</th>
              <th className="py-1 pr-3">{t('runs.analyzed')}</th>
              <th className="py-1 pr-3">{t('runs.skipped')}</th>
              <th className="py-1 pr-3">{t('runs.findings')}</th>
              <th className="py-1 pr-3">{t('runs.errors')}</th>
              <th className="py-1 pr-3">{t('runs.transcript')}</th>
            </tr>
          </thead>
          <tbody>
            {runs.map((r, i) => (
              <tr key={i} className="border-b border-[var(--color-border)]">
                <td className="py-1 pr-3">{relativeTime(r.at)}</td>
                <td className="py-1 pr-3">
                  {t('runs.seconds', { value: (r.durationMs / 1000).toFixed(1) })}
                </td>
                <td className="py-1 pr-3">{r.sessions}</td>
                <td className="py-1 pr-3">{r.analyzed}</td>
                <td className="py-1 pr-3">{r.skipped}</td>
                <td className="py-1 pr-3">{r.findings}</td>
                <td className={`py-1 pr-3 ${r.errors > 0 ? 'text-[var(--color-danger)]' : ''}`}>
                  {r.errors}
                </td>
                <td className="py-1 pr-3">
                  {r.sessionId ? (
                    <button
                      onClick={() => openRunSession(r.sessionId!)}
                      title={t('runs.openTranscriptTitle')}
                      className="flex items-center gap-1 text-[var(--color-accent)] hover:underline"
                    >
                      <MessageSquare className="h-3.5 w-3.5" /> {t('actions.open')}
                    </button>
                  ) : (
                    <span className="text-[var(--color-text-dim)]">—</span>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {runs.length === 0 && !loading && (
          <div className="py-2 text-sm text-[var(--color-text-dim)]">{t('runs.empty')}</div>
        )}
      </div>
    </div>
  )
}
