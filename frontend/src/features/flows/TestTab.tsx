import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ExternalLink, Play } from 'lucide-react'
import type { Flow, FlowNodeEvent, FlowNodeFrame, FlowRun } from '@/types'
import { api } from '@/api'
import { Badge, Button, InfoPopover } from '@/shared/components'
import { subscribeFlowNode } from '@/shared/lib/flowNodeBus'
import { formatDurationMs } from '@/shared/lib/time'
import { NODE_CHROME } from './nodeChrome'
import type { FlowNodeType } from '@/types'

interface Props {
  flow: Flow
  onOpenSession?: (sessionId: string) => void
  onError: (msg: string) => void
}

interface LiveNode {
  key: string
  ev: FlowNodeEvent
}

// TestTab runs the flow once on a typed input in a fresh, tagged chat session
// and shows the stages light up as the turn runs through them, then the reply.
export function TestTab({ flow, onOpenSession, onError }: Props) {
  const { t } = useTranslation('flows')
  const [input, setInput] = useState('')
  const [sessionId, setSessionId] = useState<string | null>(null)
  const [frames, setFrames] = useState<LiveNode[]>([])
  const [result, setResult] = useState<FlowRun | null>(null)
  const [running, setRunning] = useState(false)
  const runIdRef = useRef<string | null>(null)

  useEffect(() => {
    if (!sessionId) return
    return subscribeFlowNode(flow.id, (frame: FlowNodeFrame) => {
      if (frame.sessionId !== sessionId) return
      runIdRef.current = frame.runId
      setFrames((prev) => {
        const key = `${frame.event.nodeId}#${frame.event.visit}`
        const i = prev.findIndex((p) => p.key === key)
        const next = [...prev]
        if (i >= 0) next[i] = { key, ev: frame.event }
        else next.push({ key, ev: frame.event })
        return next
      })
      if (frame.event.type === 'output' && frame.event.phase !== 'start') {
        api
          .getFlowRun(frame.runId)
          .then((run) => {
            setResult(run)
            setRunning(false)
          })
          .catch(() => setRunning(false))
      } else if (frame.event.phase === 'error') {
        api
          .getFlowRun(frame.runId)
          .then(setResult)
          .catch(() => undefined)
          .finally(() => setRunning(false))
      }
    })
  }, [flow.id, sessionId])

  const start = () => {
    if (!input.trim()) return
    setRunning(true)
    setFrames([])
    setResult(null)
    api
      .testFlow(flow.id, input.trim())
      .then((r) => setSessionId(r.sessionId))
      .catch((e: Error) => {
        setRunning(false)
        onError(e.message)
      })
  }

  return (
    <div className="h-full min-h-0 overflow-y-auto">
      <div className="mx-auto max-w-4xl space-y-3 p-3 md:p-4">
        <textarea
          value={input}
          onChange={(e) => setInput(e.target.value)}
          rows={4}
          placeholder={t('test.placeholder')}
          data-testid="flow-test-input"
          className="w-full rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2 text-sm focus:border-[var(--color-accent)] focus:outline-none"
          onKeyDown={(e) => {
            if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') start()
          }}
        />
        <div className="flex flex-wrap items-center gap-2">
          <Button onClick={start} disabled={running || !input.trim()} data-testid="flow-test-run">
            <Play size={13} /> {running ? t('test.running') : t('test.run')}
          </Button>
          <InfoPopover text={t('test.hint', { name: flow.agentName })} />
          {sessionId && onOpenSession && (
            <Button variant="secondary" size="md" onClick={() => onOpenSession(sessionId)}>
              <ExternalLink size={13} /> {t('runs.openSession')}
            </Button>
          )}
          {sessionId && (
            <span className="font-mono text-[10px] text-[var(--color-text-dim)]">{sessionId}</span>
          )}
        </div>
        {(frames.length > 0 || running) && (
          <ol className="space-y-1" data-testid="flow-test-frames">
            {frames.map(({ key, ev }) => {
              const chrome =
                NODE_CHROME[
                  (ev.type as FlowNodeType) in NODE_CHROME ? (ev.type as FlowNodeType) : 'llm'
                ]
              const Icon = chrome.Icon
              const tone =
                ev.phase === 'error'
                  ? 'border-[var(--color-danger)]/50'
                  : ev.phase === 'start'
                    ? 'border-[var(--color-accent)] animate-pulse'
                    : 'border-[var(--color-border)]'
              return (
                <li
                  key={key}
                  className={`rounded-md border bg-[var(--color-surface)] px-3 py-1.5 text-xs ${tone}`}
                >
                  <div className="flex items-center gap-2">
                    <Icon size={13} style={{ color: chrome.accent }} />
                    <span className="font-medium">{ev.title || ev.nodeId}</span>
                    <span className="text-[var(--color-text-dim)]">
                      · {t(`types.${ev.type}`, { defaultValue: ev.type })}
                    </span>
                    {ev.visit > 1 && (
                      <span className="text-[var(--color-text-dim)]">
                        · {t('runs.visit', { n: ev.visit })}
                      </span>
                    )}
                    {ev.edge && (
                      <span className="font-mono text-[10px] text-[var(--color-warning)]">
                        → {ev.edge}
                      </span>
                    )}
                    <span className="ml-auto font-mono text-[10px] text-[var(--color-text-dim)]">
                      {ev.phase === 'start'
                        ? t('test.stageRunning')
                        : formatDurationMs(ev.durationMs ?? 0)}
                    </span>
                  </div>
                  {ev.output && ev.phase === 'done' && (
                    <p className="mt-1 line-clamp-3 whitespace-pre-wrap text-[11px] text-[var(--color-text-dim)]">
                      {ev.output}
                    </p>
                  )}
                  {ev.error && (
                    <p className="mt-1 text-[11px] text-[var(--color-danger)]">{ev.error}</p>
                  )}
                </li>
              )
            })}
            {running && frames.length === 0 && (
              <li className="text-xs text-[var(--color-text-dim)]">{t('test.waiting')}</li>
            )}
          </ol>
        )}
        {result && (
          <div
            className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-3 text-xs"
            data-testid="flow-test-result"
          >
            <div className="mb-1 flex items-center gap-2">
              <Badge tone={result.status === 'success' ? 'success' : 'danger'}>
                {t(`status.${result.status}`)}
              </Badge>
              <span className="text-[var(--color-text-dim)]">
                {formatDurationMs(result.durationMs)} ·{' '}
                {t('runs.stepsShort', { count: result.stepCount })}
              </span>
            </div>
            <pre className="max-h-80 overflow-auto whitespace-pre-wrap break-words text-[12px]">
              {result.status === 'failure' ? result.error : result.output}
            </pre>
          </div>
        )}
      </div>
    </div>
  )
}
