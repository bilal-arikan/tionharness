import { useCallback, useEffect, useRef, useState } from 'react'
import { Bug, Loader2, Pin, RefreshCw, ThumbsDown, ThumbsUp } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import {
  sessionDecisionsApi,
  type SessionDecisions,
  type SessionDecisionEntry,
  type SessionDecisionRating,
} from '@/api/sessionDecisions'
import { Section } from './SessionDetailBits'
import { useKeyedReset } from '@/shared/lib/useKeyedReset'

interface SessionDecisionsCardProps {
  sessionId: string
  refreshKey?: number
  onError?: (message: string) => void
  onOpenDebug?: () => void
}

const controlClass =
  'inline-flex min-h-11 items-center justify-center gap-1.5 rounded-lg border border-[var(--color-border)] px-2 text-[11px] text-[var(--color-text)] transition hover:border-[var(--color-accent)] aria-pressed:border-[var(--color-accent)] aria-pressed:bg-[var(--color-surface-2)] disabled:cursor-not-allowed disabled:opacity-40'

export function SessionDecisionsCard(props: SessionDecisionsCardProps) {
  // A session switch unmounts every pending action, including its busy state.
  return <SessionDecisionsContent key={props.sessionId} {...props} />
}

function SessionDecisionsContent({
  sessionId,
  refreshKey,
  onError,
  onOpenDebug,
}: SessionDecisionsCardProps) {
  const { t } = useTranslation('sessions')
  const [state, setState] = useState<SessionDecisions | null>(null)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [mutationError, setMutationError] = useState('')
  const [retry, setRetry] = useState(0)
  const generation = useRef(0)
  const alive = useRef(true)
  const mutationPending = useRef(false)
  const errorHandler = useRef(onError)

  useEffect(() => {
    errorHandler.current = onError
  }, [onError])

  useKeyedReset(`${refreshKey}|${retry}|${busy}`, () => {
    if (!busy) setLoading(true)
  })

  useEffect(() => {
    alive.current = true
    return () => {
      alive.current = false
    }
  }, [])

  useEffect(() => {
    if (busy) return
    const controller = new AbortController()
    const current = ++generation.current
    // Keep the visible summary while refreshing, but disable writes until the
    // latest server snapshot lands. No inferred or example state is displayed.
    sessionDecisionsApi
      .read(sessionId, controller.signal)
      .then((next) => {
        if (!alive.current || controller.signal.aborted || current !== generation.current) return
        if (next.sessionId !== sessionId) throw new Error('Session decision response mismatch')
        setState(next)
        setError('')
      })
      .catch((cause: unknown) => {
        if (!alive.current || controller.signal.aborted || current !== generation.current) return
        const message = cause instanceof Error ? cause.message : String(cause)
        setError(message)
        errorHandler.current?.(message)
      })
      .finally(() => {
        if (alive.current && !controller.signal.aborted && current === generation.current)
          setLoading(false)
      })
    return () => controller.abort()
  }, [sessionId, refreshKey, retry, busy])

  const mutate = useCallback(
    async (action: () => Promise<SessionDecisions>) => {
      // The ref also guards two clicks before React commits the disabled state.
      if (mutationPending.current) return
      mutationPending.current = true
      setBusy(true)
      const current = ++generation.current
      try {
        const next = await action()
        if (!alive.current || current !== generation.current) return
        if (next.sessionId !== sessionId) throw new Error('Session decision response mismatch')
        setState(next)
        setError('')
        setMutationError('')
      } catch (cause) {
        if (!alive.current || current !== generation.current) return
        const message = cause instanceof Error ? cause.message : String(cause)
        setMutationError(message)
        errorHandler.current?.(message)
      } finally {
        mutationPending.current = false
        if (alive.current) setBusy(false)
      }
    },
    [sessionId],
  )

  const visibleError = mutationError || error
  const disabled = loading || busy || !!visibleError
  const entries = (state?.entries ?? []).slice(-8).reverse()
  const hasState =
    state &&
    (state.route ||
      entries.length > 0 ||
      state.memories?.length > 0 ||
      state.selectedSkills?.length > 0 ||
      state.selectedTools?.length > 0)

  return (
    <Section title={t('decisions.title', { defaultValue: 'Session decisions' })}>
      <div className="min-w-0 space-y-3 text-xs [overflow-wrap:anywhere]">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <span className="text-[var(--color-text-dim)]">
            {t('decisions.compactCount', {
              defaultValue: 'Compactions: {{count}}',
              count: state?.compactCount ?? 0,
            })}
          </span>
          <button
            type="button"
            className={controlClass}
            disabled={loading || busy}
            onClick={() => {
              setLoading(true)
              setMutationError('')
              setRetry((value) => value + 1)
            }}
          >
            {loading ? <Loader2 size={13} className="animate-spin" /> : <RefreshCw size={13} />}
            {t('decisions.refresh', { defaultValue: 'Refresh' })}
          </button>
        </div>
        {visibleError && (
          <p role="alert" className="text-[var(--color-danger)]">
            {visibleError}
          </p>
        )}
        {loading && !state && (
          <p role="status" className="text-[var(--color-text-dim)]">
            {t('decisions.loading', { defaultValue: 'Loading decisions…' })}
          </p>
        )}
        {!loading && !visibleError && !hasState && (
          <p className="text-[var(--color-text-dim)]">
            {t('decisions.empty', {
              defaultValue: 'No decision has been recorded for this session yet.',
            })}
          </p>
        )}
        {state?.route && (
          <div className="flex min-w-0 items-center gap-2 rounded-lg border border-[var(--color-border)] p-2">
            <div className="min-w-0 flex-1">
              <div className="text-[10px] text-[var(--color-text-dim)]">
                {t('decisions.executionModel', { defaultValue: 'Execution model' })}
              </div>
              <div>{state.route.model}</div>
              <div className="text-[10px] text-[var(--color-text-dim)]">{state.route.provider}</div>
            </div>
            <button
              type="button"
              className={controlClass}
              disabled={disabled}
              aria-pressed={state.route.pinned}
              onClick={() =>
                void mutate(() => sessionDecisionsApi.pin(sessionId, 'model', !state.route!.pinned))
              }
            >
              <Pin size={13} />
              {state.route.pinned
                ? t('decisions.unpin', { defaultValue: 'Unpin' })
                : t('decisions.pin', { defaultValue: 'Pin' })}
            </button>
          </div>
        )}
        {!!state?.selectedSkills?.length && (
          <SelectionList
            label={t('decisions.skills', { defaultValue: 'Loaded skills' })}
            values={state.selectedSkills}
          />
        )}
        {!!state?.selectedTools?.length && (
          <SelectionList
            label={t('decisions.tools', { defaultValue: 'Activated tools' })}
            values={state.selectedTools}
          />
        )}
        {!!state?.memories?.length && (
          <div className="space-y-2">
            <h4 className="font-medium text-[var(--color-text)]">
              {t('decisions.memories', { defaultValue: 'Retained context' })}
            </h4>
            {state.memories.map((memory) => (
              <div key={memory.key} className="rounded-lg border border-[var(--color-border)] p-2">
                <div className="flex items-center gap-2">
                  <div className="min-w-0 flex-1">
                    {memory.label}
                    {memory.mandatory && (
                      <span className="ml-1 text-[10px] text-[var(--color-text-dim)]">
                        {t('decisions.required', { defaultValue: 'Required' })}
                      </span>
                    )}
                  </div>
                  <button
                    type="button"
                    className={controlClass}
                    disabled={disabled || memory.mandatory}
                    aria-pressed={memory.pinned || memory.mandatory}
                    aria-label={t('decisions.pinContext', {
                      defaultValue: 'Pin context: {{label}}',
                      label: memory.label,
                    })}
                    onClick={() =>
                      void mutate(() =>
                        sessionDecisionsApi.pin(sessionId, memory.key, !memory.pinned),
                      )
                    }
                  >
                    <Pin size={13} />
                    {memory.mandatory
                      ? t('decisions.required', { defaultValue: 'Required' })
                      : memory.pinned
                        ? t('decisions.unpin', { defaultValue: 'Unpin' })
                        : t('decisions.pin', { defaultValue: 'Pin' })}
                  </button>
                </div>
                {memory.text && (
                  <details className="mt-1">
                    <summary className="flex min-h-11 cursor-pointer items-center text-[var(--color-text-dim)]">
                      {t('decisions.preview', { defaultValue: 'View context' })}
                    </summary>
                    <p className="max-h-40 overflow-auto whitespace-pre-wrap text-[11px]">
                      {memory.text}
                    </p>
                  </details>
                )}
                {memory.lastReminded > 0 && (
                  <p className="mt-1 text-[10px] text-[var(--color-text-dim)]">
                    {t('decisions.reminded', {
                      defaultValue: 'Last reminder at compaction {{count}}',
                      count: memory.lastReminded,
                    })}
                  </p>
                )}
              </div>
            ))}
          </div>
        )}
        {entries.length > 0 && (
          <div className="space-y-2">
            <p className="text-[var(--color-text-dim)]" title={t('decisions.evidenceHint')}>
              {t('decisions.evidence', {
                applied: state?.entries.filter((entry) => entry.status === 'applied').length ?? 0,
                observed: state?.entries.filter((entry) => entry.status === 'observed').length ?? 0,
                fallback: state?.entries.filter((entry) => entry.status === 'fallback').length ?? 0,
                helpful: state?.feedback.filter((item) => item.rating === 'helpful').length ?? 0,
                correction:
                  state?.feedback.filter((item) => item.rating === 'correction').length ?? 0,
              })}
            </p>
            <h4 className="font-medium text-[var(--color-text)]">
              {t('decisions.recent', { defaultValue: 'Recent decisions' })}
            </h4>
            {entries.map((entry) => (
              <DecisionDetails
                key={entry.id}
                entry={entry}
                rating={
                  [...(state?.feedback ?? [])]
                    .reverse()
                    .find((item) => item.decisionId === entry.id)?.rating
                }
                disabled={disabled}
                onFeedback={(rating) =>
                  void mutate(() => sessionDecisionsApi.feedback(sessionId, entry.id, rating))
                }
                onOpenDebug={onOpenDebug}
              />
            ))}
          </div>
        )}
      </div>
    </Section>
  )
}

function SelectionList({ label, values }: { label: string; values: string[] }) {
  return (
    <div>
      <h4 className="mb-1 text-[10px] text-[var(--color-text-dim)]">{label}</h4>
      <ul className="flex flex-wrap gap-1">
        {values.map((value) => (
          <li
            key={value}
            className="max-w-full rounded-md border border-[var(--color-border)] px-1.5 py-1 text-[11px]"
          >
            {value}
          </li>
        ))}
      </ul>
    </div>
  )
}

function DecisionDetails({
  entry,
  rating,
  disabled,
  onFeedback,
  onOpenDebug,
}: {
  entry: SessionDecisionEntry
  rating?: SessionDecisionRating
  disabled: boolean
  onFeedback: (rating: SessionDecisionRating) => void
  onOpenDebug?: () => void
}) {
  const { t } = useTranslation('sessions')
  return (
    <details className="min-w-0 rounded-lg border border-[var(--color-border)] px-2">
      <summary className="min-h-11 cursor-pointer py-2 text-[11px]">
        <span className="font-medium">
          {t(`decisions.authority.${entry.authority}`, { defaultValue: entry.authority })}
        </span>
        <span className="ml-1 text-[var(--color-text-dim)]">
          · {t(`decisions.status.${entry.status}`, { defaultValue: entry.status })}
        </span>
      </summary>
      <div className="space-y-2 pb-2 text-[11px]">
        <div className="text-[var(--color-text-dim)]">
          {new Date(entry.at).toLocaleString()} ·{' '}
          {t(`decisions.mode.${entry.mode}`, { defaultValue: entry.mode })}
        </div>
        {[
          ['recommended', entry.recommended],
          ['applied', entry.applied],
          ['baseline', entry.baseline],
        ].map(([key, value]) =>
          value ? (
            <p key={key}>
              <span className="text-[var(--color-text-dim)]">
                {t(`decisions.${key}`, { defaultValue: key })}:{' '}
              </span>
              {value}
            </p>
          ) : null,
        )}
        {!!entry.items?.length && (
          <ul className="space-y-1">
            {entry.items.map((item, index) => (
              <li key={`${item.key}:${index}`}>
                {item.label} · {t(`decisions.action.${item.action}`, { defaultValue: item.action })}
                {typeof item.strength === 'number' &&
                  Number.isFinite(item.strength) &&
                  ` · ${Math.round(item.strength * 100)}%`}
              </li>
            ))}
          </ul>
        )}
        {entry.error && <p className="text-[var(--color-danger)]">{entry.error}</p>}
        {entry.traceId && (
          <div className="rounded bg-[var(--color-surface-2)] p-2">
            <div className="text-[10px] text-[var(--color-text-dim)]">
              {t('decisions.trace', { defaultValue: 'Decision trace' })}
            </div>
            <code className="select-text text-[10px]">{entry.traceId}</code>
            {onOpenDebug && (
              <button type="button" className={`${controlClass} mt-1`} onClick={onOpenDebug}>
                <Bug size={13} />
                {t('decisions.openDebug', { defaultValue: 'Open debug' })}
              </button>
            )}
          </div>
        )}
        <div className="flex flex-wrap gap-1">
          <button
            type="button"
            className={controlClass}
            disabled={disabled}
            aria-pressed={rating === 'helpful'}
            onClick={() => onFeedback('helpful')}
          >
            <ThumbsUp size={13} />
            {t('decisions.helpful', { defaultValue: 'Helpful' })}
          </button>
          <button
            type="button"
            className={controlClass}
            disabled={disabled}
            aria-pressed={rating === 'correction'}
            onClick={() => onFeedback('correction')}
          >
            <ThumbsDown size={13} />
            {t('decisions.correction', { defaultValue: 'Needs correction' })}
          </button>
        </div>
      </div>
    </details>
  )
}
