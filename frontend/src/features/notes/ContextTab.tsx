import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ExternalLink, RefreshCw } from 'lucide-react'
import { api } from '@/api'
import { SIGNAL_NOTES } from '@/app/eventToRefreshSignals'
import { Button, EmptyState, KeyValueRow, LoadingState } from '@/shared/components'
import { useRefreshTrigger } from '@/shared/hooks/useRefreshTrigger'
import { relativeTime } from '@/shared/lib/time'
import type { AwarenessSeen, Session } from '@/types'
import { CompositionView } from './CompositionView'

interface Props {
  onError: (msg: string) => void
  onOpenSession?: (sessionId: string) => void
}

const selectCls =
  'min-w-0 max-w-full rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm'

// ContextTab is the "what did the agent see" view: pick a recent session and
// inspect the brief it started with, the last per-turn block, the last pulse
// line and how many turns the layer has served it.
export function ContextTab({ onError, onOpenSession }: Props) {
  const { t } = useTranslation('notes')
  const tick = useRefreshTrigger(SIGNAL_NOTES)
  const [sessions, setSessions] = useState<Session[]>([])
  const [sessionsLoading, setSessionsLoading] = useState(true)
  const [selected, setSelected] = useState('')
  const [nonce, setNonce] = useState(0)
  // The answer records the request it belongs to; loading is derived from the
  // gap so the effect only sets state from its callbacks.
  const [answer, setAnswer] = useState<{ key: string; seen: AwarenessSeen | null }>({
    key: '',
    seen: null,
  })

  useEffect(() => {
    const ctl = new AbortController()
    api
      .listSessions({ limit: 50, sort: 'updated_desc' }, ctl.signal)
      .then((page) => {
        if (!ctl.signal.aborted) setSessions(page.items)
      })
      .catch((e) => {
        if (!ctl.signal.aborted) onError((e as Error).message)
      })
      .finally(() => {
        if (!ctl.signal.aborted) setSessionsLoading(false)
      })
    return () => ctl.abort()
  }, [onError])

  const seenKey = selected ? `${selected}|${tick}|${nonce}` : ''
  useEffect(() => {
    if (!selected) return
    const ctl = new AbortController()
    api
      .sessionAwareness(selected, ctl.signal)
      .then((seen) => {
        if (!ctl.signal.aborted) setAnswer({ key: seenKey, seen })
      })
      .catch((e) => {
        if (ctl.signal.aborted) return
        setAnswer({ key: seenKey, seen: null })
        onError((e as Error).message)
      })
    return () => ctl.abort()
  }, [selected, seenKey, onError])

  const loading = !!selected && answer.key !== seenKey
  const seen = answer.key === seenKey ? answer.seen : null

  const current = seen && seen.sessionId === selected ? seen : null

  return (
    <div className="flex h-full min-h-0 flex-col gap-3 overflow-y-auto p-4">
      <div className="flex flex-wrap items-center gap-2">
        <label className="flex min-w-0 items-center gap-2 text-xs text-[var(--color-text-dim)]">
          <span className="shrink-0">{t('context.session')}</span>
          <select
            data-testid="context-session"
            className={selectCls}
            value={selected}
            onChange={(e) => setSelected(e.target.value)}
            disabled={sessionsLoading}
          >
            <option value="">{t('context.pickSession')}</option>
            {sessions.map((s) => (
              <option key={s.id} value={s.id}>
                {(s.title || s.id).slice(0, 80)} · {relativeTime(s.updatedAt || s.createdAt)}
              </option>
            ))}
          </select>
        </label>
        {selected && (
          <>
            <Button
              size="sm"
              variant="secondary"
              onClick={() => setNonce((n) => n + 1)}
              disabled={loading}
            >
              <RefreshCw size={13} className={loading ? 'animate-spin' : ''} />{' '}
              {t('context.refresh')}
            </Button>
            {onOpenSession && (
              <Button size="sm" variant="secondary" onClick={() => onOpenSession(selected)}>
                <ExternalLink size={13} /> {t('actions.openSession')}
              </Button>
            )}
          </>
        )}
      </div>

      {!selected ? (
        <EmptyState
          title={t('context.selectPrompt')}
          hint={t('context.selectHint')}
          className="flex-1 justify-center"
        />
      ) : loading && !current ? (
        <LoadingState label={t('context.loading')} />
      ) : current ? (
        <>
          <div className="grid grid-cols-1 gap-x-6 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 sm:grid-cols-2">
            <KeyValueRow label={t('context.turns')} value={String(current.turns)} />
            <KeyValueRow
              label={t('context.digestState')}
              value={current.digest ? t('context.digestWritten') : t('context.digestNone')}
            />
            <div className="py-0.5 text-xs sm:col-span-2">
              <span className="text-[var(--color-text-dim)]">{t('context.lastPulse')}</span>
              {current.lastPulse ? (
                <p
                  data-testid="context-pulse"
                  className="mt-0.5 rounded bg-[var(--color-bg)] px-2 py-1 font-mono text-[11px]"
                >
                  {current.lastPulse}
                </p>
              ) : (
                <span className="ml-2 text-[var(--color-text-dim)]">{t('context.noPulse')}</span>
              )}
            </div>
          </div>
          <CompositionView
            title={t('context.brief')}
            composition={current.brief}
            emptyLabel={t('context.noBrief')}
          />
          <CompositionView
            title={t('context.lastTurn')}
            composition={current.lastTurn}
            emptyLabel={t('context.noTurn')}
          />
        </>
      ) : null}
    </div>
  )
}
