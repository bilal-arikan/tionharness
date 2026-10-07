import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { AlertTriangle, ExternalLink, FileText, Lightbulb, ListTodo } from 'lucide-react'
import { api } from '@/api'
import { SIGNAL_NOTES } from '@/app/eventToRefreshSignals'
import { Badge, Button, EmptyState, KeyValueRow, LoadingState } from '@/shared/components'
import { SELECTED_ITEM_CLS } from '@/shared/components/SidebarChrome'
import { useRefreshTrigger } from '@/shared/hooks/useRefreshTrigger'
import { formatDuration, relativeTime } from '@/shared/lib/time'
import type { Digest, DigestIndexEntry } from '@/types'
import { digestFlags } from './notesHelpers'

interface Props {
  onError: (msg: string) => void
  // agent id → display name, for the row's agent label.
  agentNames: Record<string, string>
  onOpenSession?: (sessionId: string) => void
}

// DigestsTab lists the end-of-turn digests the awareness layer wrote (newest
// first) and shows one verbatim: the same text other sessions' briefs quote.
export function DigestsTab({ onError, agentNames, onOpenSession }: Props) {
  const { t } = useTranslation('notes')
  const tick = useRefreshTrigger(SIGNAL_NOTES)
  // Both fetches record which request they answered; "loading" is the gap
  // between the live request and the last answer, so the effects only set
  // state from their callbacks.
  const [index, setIndex] = useState<{ tick: number; entries: DigestIndexEntry[] }>({
    tick: -1,
    entries: [],
  })
  const [selected, setSelected] = useState<string | null>(null)
  const [answer, setAnswer] = useState<{ key: string; digest: Digest | null }>({
    key: '',
    digest: null,
  })

  useEffect(() => {
    const ctl = new AbortController()
    api
      .listDigests(200, ctl.signal)
      .then((entries) => {
        if (!ctl.signal.aborted) setIndex({ tick, entries })
      })
      .catch((e) => {
        if (ctl.signal.aborted) return
        onError((e as Error).message)
        setIndex((i) => ({ ...i, tick }))
      })
    return () => ctl.abort()
  }, [tick, onError])

  const digestKey = selected ? `${selected}|${tick}` : ''
  useEffect(() => {
    if (!selected) return
    const ctl = new AbortController()
    api
      .sessionDigest(selected, ctl.signal)
      .then((digest) => {
        if (!ctl.signal.aborted) setAnswer({ key: digestKey, digest })
      })
      .catch((e) => {
        if (ctl.signal.aborted) return
        setAnswer({ key: digestKey, digest: null })
        onError((e as Error).message)
      })
    return () => ctl.abort()
  }, [selected, digestKey, onError])

  const entries = index.entries
  const loading = index.tick !== tick
  const digestLoading = !!selected && answer.key !== digestKey
  const digest = answer.key === digestKey ? answer.digest : null

  const current = digest && digest.sessionId === selected ? digest : null

  return (
    <div className="flex h-full min-h-0 flex-col md:flex-row">
      <div className="max-h-[40%] shrink-0 overflow-y-auto border-b border-[var(--color-border)] md:max-h-none md:w-96 md:border-b-0 md:border-r">
        {loading && entries.length === 0 ? (
          <LoadingState label={t('digests.loading')} />
        ) : entries.length === 0 ? (
          <EmptyState icon={FileText} title={t('digests.empty')} hint={t('digests.emptyHint')} />
        ) : (
          <ul className="flex flex-col gap-1 p-2" data-testid="digest-list">
            {entries.map((e) => {
              const active = e.sessionId === selected
              const flags = digestFlags(e)
              return (
                <li key={`${e.sessionId}:${e.hash}`}>
                  <button
                    type="button"
                    data-testid="digest-row"
                    onClick={() => setSelected(e.sessionId)}
                    aria-current={active ? 'true' : undefined}
                    className={`w-full rounded-lg px-3 py-2 text-left transition ${
                      active ? SELECTED_ITEM_CLS : 'hover:bg-[var(--color-surface-2)]'
                    }`}
                  >
                    <div className="flex items-start gap-2">
                      <span className="min-w-0 flex-1 truncate text-sm font-medium" title={e.title}>
                        {e.title || e.sessionId}
                      </span>
                      <span className="shrink-0 text-[10px] text-[var(--color-text-dim)]">
                        {relativeTime(e.at)}
                      </span>
                    </div>
                    <div className="mt-0.5 flex flex-wrap items-center gap-1 text-[10px] text-[var(--color-text-dim)]">
                      {e.agentId && <span>{agentNames[e.agentId] ?? e.agentId}</span>}
                      <code className="opacity-70">{e.sessionId}</code>
                      {flags.includes('errors') && (
                        <Badge tone="danger" className="gap-0.5">
                          <AlertTriangle size={10} />{' '}
                          {t('digests.errors', { count: e.errors ?? 0 })}
                        </Badge>
                      )}
                      {flags.includes('openLoops') && (
                        <Badge tone="warning" className="gap-0.5">
                          <ListTodo size={10} />{' '}
                          {t('digests.openLoops', { count: e.openLoops ?? 0 })}
                        </Badge>
                      )}
                      {flags.includes('suggestNote') && (
                        <Badge tone="accent" className="gap-0.5">
                          <Lightbulb size={10} /> {t('digests.suggestNote')}
                        </Badge>
                      )}
                    </div>
                    {e.line && (
                      <p className="mt-1 line-clamp-2 text-xs text-[var(--color-text-dim)]">
                        {e.line}
                      </p>
                    )}
                  </button>
                </li>
              )
            })}
          </ul>
        )}
      </div>

      <div className="min-h-0 min-w-0 flex-1 overflow-y-auto p-4">
        {!selected ? (
          <EmptyState title={t('digests.selectPrompt')} className="h-full justify-center" />
        ) : digestLoading && !current ? (
          <LoadingState label={t('digests.loading')} />
        ) : !current ? (
          <EmptyState title={t('digests.notFound')} />
        ) : (
          <div className="flex flex-col gap-3">
            <div className="flex flex-wrap items-start justify-between gap-2">
              <div className="min-w-0 flex-1">
                <h2 className="text-base font-semibold">{current.title || current.sessionId}</h2>
                <div className="mt-0.5 flex flex-wrap items-center gap-2 text-xs text-[var(--color-text-dim)]">
                  {current.agentName && <span>{current.agentName}</span>}
                  <code>{current.sessionId}</code>
                  {current.kind && <Badge>{current.kind}</Badge>}
                  {current.state && <Badge>{current.state}</Badge>}
                  {current.suggestNote && (
                    <Badge tone="accent" className="gap-0.5">
                      <Lightbulb size={10} /> {t('digests.suggestNote')}
                    </Badge>
                  )}
                </div>
              </div>
              {onOpenSession && (
                <Button
                  size="sm"
                  variant="secondary"
                  onClick={() => onOpenSession(current.sessionId)}
                  data-testid="digest-open-session"
                >
                  <ExternalLink size={13} /> {t('actions.openSession')}
                </Button>
              )}
            </div>

            <div className="grid grid-cols-1 gap-x-6 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 sm:grid-cols-2">
              <KeyValueRow label={t('digests.facts.messages')} value={String(current.messages)} />
              <KeyValueRow
                label={t('digests.facts.toolCalls')}
                value={
                  current.toolErrors > 0
                    ? t('digests.facts.toolCallsWithErrors', {
                        calls: current.toolCalls,
                        errors: current.toolErrors,
                      })
                    : String(current.toolCalls)
                }
              />
              {current.durationSec ? (
                <KeyValueRow
                  label={t('digests.facts.duration')}
                  value={formatDuration(current.durationSec)}
                />
              ) : null}
              <KeyValueRow
                label={t('digests.facts.todo')}
                value={t('digests.facts.todoValue', {
                  done: current.todo.done,
                  total: current.todo.total,
                  inProgress: current.todo.inProgress,
                })}
              />
              {(current.children ?? 0) > 0 && (
                <KeyValueRow
                  label={t('digests.facts.children')}
                  value={
                    (current.childrenFailed ?? 0) > 0
                      ? t('digests.facts.childrenWithFailed', {
                          count: current.children ?? 0,
                          failed: current.childrenFailed ?? 0,
                        })
                      : String(current.children ?? 0)
                  }
                />
              )}
              {(current.stuckTurns ?? 0) > 0 && (
                <KeyValueRow
                  label={t('digests.facts.stuckTurns')}
                  value={String(current.stuckTurns ?? 0)}
                />
              )}
              {current.waitingAsk && (
                <KeyValueRow label={t('digests.facts.waitingAsk')} value={t('digests.yes')} />
              )}
              {current.lastError && (
                <div className="py-0.5 text-xs sm:col-span-2">
                  <span className="text-[var(--color-text-dim)]">
                    {t('digests.facts.lastError')}
                  </span>
                  <p className="mt-0.5 break-words font-mono text-[var(--color-danger)]">
                    {current.lastError}
                  </p>
                </div>
              )}
            </div>

            {(current.openLoops?.length ?? 0) > 0 && (
              <div>
                <h3 className="mb-1 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
                  {t('digests.openLoopsTitle')}
                </h3>
                <ul className="list-disc space-y-0.5 pl-5 text-xs">
                  {current.openLoops!.map((loop, i) => (
                    <li key={i}>{loop}</li>
                  ))}
                </ul>
              </div>
            )}

            <pre
              data-testid="digest-text"
              className="overflow-auto whitespace-pre-wrap rounded-md bg-[var(--color-surface-2)] p-3 text-xs leading-relaxed text-[var(--color-text)]"
            >
              {current.text}
            </pre>
          </div>
        )}
      </div>
    </div>
  )
}
