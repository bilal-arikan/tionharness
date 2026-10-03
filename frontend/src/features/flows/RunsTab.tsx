import { useEffect, useState } from 'react'
import { useKeyedReset } from '@/shared/lib/useKeyedReset'
import { useTranslation } from 'react-i18next'
import { ArrowLeft, ExternalLink, Star, ThumbsDown, ThumbsUp, Trash2 } from 'lucide-react'
import type { Flow, FlowRun, FlowRunStep } from '@/types'
import { api } from '@/api'
import { Badge, EmptyState, type BadgeTone } from '@/shared/components'
import { formatDurationMs, relativeTime, fullDateTime } from '@/shared/lib/time'
import { tokens as fmtTokens } from '@/shared/lib/format'
import { NODE_CHROME } from './nodeChrome'
import type { FlowNodeType } from '@/types'

interface Props {
  flow: Flow
  runs: FlowRun[]
  loading: boolean
  stacked: boolean
  onReload: () => void
  onOpenSession?: (sessionId: string) => void
  onError: (msg: string) => void
}

function statusTone(status: FlowRun['status']): BadgeTone {
  return status === 'success' ? 'success' : status === 'failure' ? 'danger' : 'accent'
}

function excerpt(s: string | undefined, n = 120): string {
  if (!s) return ''
  const one = s.replace(/\s+/g, ' ').trim()
  return one.length > n ? one.slice(0, n - 1) + '…' : one
}

// GradeBadge shows the decision model's 1..5 grade of a run's reply.
function GradeBadge({ run }: { run: FlowRun }) {
  const { t } = useTranslation('flows')
  if (!run.grade) return null
  const tone =
    run.grade >= 4
      ? 'var(--color-success)'
      : run.grade >= 3
        ? 'var(--color-warning)'
        : 'var(--color-danger)'
  return (
    <span
      className="inline-flex items-center gap-0.5 rounded px-1 py-0.5 text-[10px] font-medium"
      style={{ color: tone, background: `color-mix(in srgb, ${tone} 14%, transparent)` }}
      title={t('runs.gradeTitle', { pct: Math.round((run.gradeConfidence ?? 0) * 100) })}
      data-testid="flow-run-grade"
    >
      <Star size={10} /> {t('runs.grade', { n: run.grade })}
    </span>
  )
}

function StepRow({ step }: { step: FlowRunStep }) {
  const { t } = useTranslation('flows')
  const [open, setOpen] = useState(false)
  const chrome =
    NODE_CHROME[(step.type as FlowNodeType) in NODE_CHROME ? (step.type as FlowNodeType) : 'llm']
  const Icon = chrome.Icon
  const hasBody = !!step.input || !!step.output || !!step.error || !!step.detail
  return (
    <li className="rounded-md border border-[var(--color-border)] bg-[var(--color-surface)]">
      <button
        type="button"
        onClick={() => hasBody && setOpen((o) => !o)}
        className={`flex w-full items-center gap-2 px-3 py-1.5 text-left text-xs ${hasBody ? 'hover:bg-[var(--color-surface-2)]' : 'cursor-default'}`}
      >
        <span className="w-5 shrink-0 font-mono text-[10px] text-[var(--color-text-dim)]">
          {step.index}
        </span>
        <Icon
          size={13}
          style={{ color: step.error ? 'var(--color-danger)' : chrome.accent }}
          className="shrink-0"
        />
        <span className="min-w-0 flex-1 truncate">
          {step.title || step.nodeId}
          <span className="ml-1 text-[var(--color-text-dim)]">
            · {t(`types.${step.type}`, { defaultValue: step.type })}
          </span>
          {step.visit > 1 && (
            <span className="ml-1 text-[var(--color-text-dim)]">
              · {t('runs.visit', { n: step.visit })}
            </span>
          )}
          {step.edge && (
            <span className="ml-1 font-mono text-[10px] text-[var(--color-warning)]">
              → {step.edge}
            </span>
          )}
          {step.detail && (
            <span className="ml-1 text-[10px] text-[var(--color-text-dim)]">· {step.detail}</span>
          )}
        </span>
        <span className="shrink-0 font-mono text-[10px] text-[var(--color-text-dim)]">
          {formatDurationMs(step.durationMs)}
        </span>
        {hasBody && <span className="shrink-0 text-[10px] opacity-50">{open ? '▾' : '▸'}</span>}
      </button>
      {open && (
        <div className="space-y-2 border-t border-[var(--color-border)] px-3 py-2 text-[11px]">
          {step.detail && (
            <div>
              <div className="mb-0.5 font-medium text-[var(--color-text-dim)]">
                {t('runs.detail')}
              </div>
              <p className="whitespace-pre-wrap break-words">{step.detail}</p>
            </div>
          )}
          {step.input && (
            <div>
              <div className="mb-0.5 font-medium text-[var(--color-text-dim)]">
                {t('runs.stepInput')}
              </div>
              <pre className="max-h-48 overflow-auto whitespace-pre-wrap break-words rounded bg-[var(--color-bg)] p-2 font-mono">
                {step.input}
              </pre>
            </div>
          )}
          {step.output && (
            <div>
              <div className="mb-0.5 font-medium text-[var(--color-text-dim)]">
                {t('runs.stepOutput')}
              </div>
              <pre className="max-h-64 overflow-auto whitespace-pre-wrap break-words rounded bg-[var(--color-bg)] p-2 font-mono">
                {step.output}
              </pre>
            </div>
          )}
          {step.error && <div className="text-[var(--color-danger)]">{step.error}</div>}
        </div>
      )}
    </li>
  )
}

function RunDetail({
  run,
  onBack,
  onOpenSession,
  onDelete,
  stacked,
}: {
  run: FlowRun
  onBack: () => void
  onOpenSession?: (sessionId: string) => void
  onDelete: () => void
  stacked: boolean
}) {
  const { t } = useTranslation('flows')
  const steps = run.steps ?? []
  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex flex-wrap items-center gap-2 border-b border-[var(--color-border)] px-4 py-2 text-xs">
        {stacked && (
          <button
            type="button"
            onClick={onBack}
            className="rounded p-1 hover:bg-[var(--color-surface-2)]"
            title={t('runs.back')}
          >
            <ArrowLeft size={14} />
          </button>
        )}
        <Badge tone={statusTone(run.status)}>{t(`status.${run.status}`)}</Badge>
        <span className="font-mono text-[var(--color-text-dim)]">{run.id}</span>
        <span className="text-[var(--color-text-dim)]">{t('version', { v: run.version })}</span>
        <span className="text-[var(--color-text-dim)]">{formatDurationMs(run.durationMs)}</span>
        <span className="text-[var(--color-text-dim)]">
          {t('runs.tokens', { n: fmtTokens(run.usage.inputTokens + run.usage.outputTokens) })}
        </span>
        <span className="text-[var(--color-text-dim)]">
          {t('runs.llmCalls', { count: run.usage.llmCalls })}
        </span>
        <GradeBadge run={run} />
        {run.feedback ? (
          run.feedback > 0 ? (
            <ThumbsUp size={13} className="text-[var(--color-success)]" />
          ) : (
            <ThumbsDown size={13} className="text-[var(--color-danger)]" />
          )
        ) : null}
        <span className="ml-auto flex items-center gap-1">
          {run.sessionId && onOpenSession && (
            <button
              type="button"
              onClick={() => onOpenSession(run.sessionId!)}
              className="flex items-center gap-1 rounded border border-[var(--color-border)] px-2 py-1 hover:bg-[var(--color-surface-2)]"
            >
              <ExternalLink size={12} /> {t('runs.openSession')}
            </button>
          )}
          <button
            type="button"
            onClick={onDelete}
            className="rounded p-1 text-[var(--color-text-dim)] hover:text-[var(--color-danger)]"
            title={t('runs.delete')}
          >
            <Trash2 size={13} />
          </button>
        </span>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto px-4 py-3">
        <div className="mb-3 grid gap-2 md:grid-cols-2">
          <div>
            <div className="mb-0.5 text-[11px] font-medium text-[var(--color-text-dim)]">
              {t('runs.input')}
            </div>
            <pre className="max-h-40 overflow-auto whitespace-pre-wrap break-words rounded bg-[var(--color-surface)] p-2 text-[11px]">
              {run.input}
            </pre>
          </div>
          <div>
            <div className="mb-0.5 text-[11px] font-medium text-[var(--color-text-dim)]">
              {run.status === 'failure' ? t('runs.error') : t('runs.output')}
            </div>
            <pre
              className={`max-h-40 overflow-auto whitespace-pre-wrap break-words rounded bg-[var(--color-surface)] p-2 text-[11px] ${run.status === 'failure' ? 'text-[var(--color-danger)]' : ''}`}
            >
              {run.status === 'failure' ? run.error : run.output}
            </pre>
          </div>
        </div>
        <div className="mb-1 text-[11px] font-medium text-[var(--color-text-dim)]">
          {t('runs.steps', { count: steps.length })}
        </div>
        {steps.length === 0 ? (
          <p className="text-xs text-[var(--color-text-dim)]">{t('runs.noSteps')}</p>
        ) : (
          <ol className="space-y-1">
            {steps.map((s) => (
              <StepRow key={`${s.index}-${s.nodeId}`} step={s} />
            ))}
          </ol>
        )}
      </div>
    </div>
  )
}

// RunsTab lists one run per turn and opens the node trace of the selected one.
// Two columns on wide landscape screens; a list → detail stack elsewhere.
export function RunsTab({ flow, runs, loading, stacked, onReload, onOpenSession, onError }: Props) {
  const { t } = useTranslation('flows')
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [detail, setDetail] = useState<FlowRun | null>(null)

  useKeyedReset(selectedId, () => setDetail(null))
  useEffect(() => {
    if (!selectedId) return
    let cancelled = false
    api
      .getFlowRun(selectedId)
      .then((r) => {
        if (!cancelled) setDetail(r)
      })
      .catch((e: Error) => {
        if (!cancelled) onError(e.message)
      })
    return () => {
      cancelled = true
    }
    // The selected run's row (status) changes as it finishes: refetch then too.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedId, runs.find((r) => r.id === selectedId)?.status, onError])

  const list = (
    <ul className="divide-y divide-[var(--color-border)]" data-testid="flow-runs-list">
      {runs.map((r) => (
        <li key={r.id}>
          <button
            type="button"
            onClick={() => setSelectedId(r.id)}
            className={`w-full px-3 py-2 text-left text-xs hover:bg-[var(--color-surface-2)] ${selectedId === r.id ? 'bg-[var(--color-accent-soft)]' : ''}`}
          >
            <div className="flex items-center gap-2">
              <Badge tone={statusTone(r.status)}>{t(`status.${r.status}`)}</Badge>
              <GradeBadge run={r} />
              <span className="text-[var(--color-text-dim)]" title={fullDateTime(r.createdAt)}>
                {relativeTime(r.createdAt)}
              </span>
              <span className="ml-auto font-mono text-[10px] text-[var(--color-text-dim)]">
                {t('version', { v: r.version })} · {formatDurationMs(r.durationMs)} ·{' '}
                {t('runs.stepsShort', { count: r.stepCount })}
              </span>
            </div>
            <div className="mt-1 truncate text-[var(--color-text)]">{excerpt(r.input)}</div>
          </button>
        </li>
      ))}
    </ul>
  )

  if (!loading && runs.length === 0) {
    return (
      <EmptyState title={t('runs.empty')}>
        {t('runs.emptyHint', { name: flow.agentName })}
      </EmptyState>
    )
  }
  const detailPane = detail ? (
    <RunDetail
      run={detail}
      stacked={stacked}
      onBack={() => setSelectedId(null)}
      onOpenSession={onOpenSession}
      onDelete={() => {
        api
          .deleteFlowRun(detail.id)
          .then(() => {
            setSelectedId(null)
            onReload()
          })
          .catch((e: Error) => onError(e.message))
      }}
    />
  ) : (
    <EmptyState title={t('runs.pick')} />
  )
  if (stacked) {
    return (
      <div className="h-full min-h-0 overflow-y-auto">
        {selectedId && detail ? detailPane : list}
      </div>
    )
  }
  return (
    <div className="flex h-full min-h-0">
      <div className="w-80 shrink-0 overflow-y-auto border-r border-[var(--color-border)]">
        {list}
      </div>
      <div className="min-w-0 flex-1">{detailPane}</div>
    </div>
  )
}
