import { useEffect, useState } from 'react'
import { Bug, ChevronDown, ChevronRight, Loader2, AlertTriangle, Info } from 'lucide-react'
import { api } from '@/api'
import { InfoPopover } from '@/shared/components'
import { useKeyedReset } from '@/shared/lib/useKeyedReset'
import type { SessionDebugSummary, SessionDebugEvent } from '@/types'
import { modelDisplayName } from '@/shared/lib/modelLabel'
import { SessionFlowViz } from './viz/SessionFlowViz'
import { toolDisplayName } from './viz/flowVizData'
import { formatTime } from '@/shared/lib/intl'
import { formatBytes as fmtBytes, tokens as fmtTok, usd as fmtUsd } from '@/shared/lib/format'
import { formatDurationMs as fmtDur } from '@/shared/lib/time'
import { useTranslation } from 'react-i18next'
import { i18next } from '@/i18n'

// SessionDebugCard renders the per-session DEBUG journal (parallel observability
// stream): turn timings, token spend by model, per-tool latency/size/errors,
// compaction/recovery counts — plus an expandable raw-event log. It is the UI
// counterpart to the read_session_debug agent tool and the /debug endpoint, and
// the data an agent reads to self-diagnose and optimise. Self-contained: it
// fetches its own summary and (lazily) the raw events.
export function SessionDebugCard({
  sessionId,
  refreshKey,
  agentNames = {},
  alwaysOpen = false,
}: {
  sessionId: string
  refreshKey?: number
  // agentId → display name, used to label the workflow visualizations' agent
  // lanes/nodes. Empty is fine (falls back to a generic label).
  agentNames?: Record<string, string>
  // When true (e.g. rendered inside the dedicated Debug modal) the card is always
  // expanded and its collapse header is hidden — the modal supplies the title.
  alwaysOpen?: boolean
}) {
  const { t } = useTranslation('sessions')
  const [sum, setSum] = useState<SessionDebugSummary | null>(null)
  // Whole-card fold (collapsed by default — debug is secondary; the header line
  // still shows a one-glance summary). Persisted so the choice sticks.
  const [open, setOpen] = useState(
    () => alwaysOpen || localStorage.getItem('tionharness.debugCardOpen') === '1',
  )
  const toggleOpen = () =>
    setOpen((v) => {
      const next = !v
      localStorage.setItem('tionharness.debugCardOpen', next ? '1' : '0')
      return next
    })
  const [expanded, setExpanded] = useState(false)
  const [events, setEvents] = useState<SessionDebugEvent[] | null>(null)
  const [loadingEvents, setLoadingEvents] = useState(false)
  const [typeFilter, setTypeFilter] = useState('')

  useEffect(() => {
    let alive = true
    api
      .sessionDebugSummary(sessionId)
      .then((d) => alive && setSum(d))
      .catch(() => alive && setSum(null))
    return () => {
      alive = false
    }
  }, [sessionId, refreshKey])

  // Raw events are fetched lazily on first expand and when the filter changes;
  // each trigger re-arms the spinner before the fetch lands via callbacks.
  useKeyedReset(`${expanded}|${sessionId}|${typeFilter}|${refreshKey}`, () => {
    if (expanded) setLoadingEvents(true)
  })
  useEffect(() => {
    if (!expanded) return
    let alive = true
    api
      .sessionDebugEvents(sessionId, typeFilter, 200)
      .then((e) => alive && setEvents(e))
      .catch(() => alive && setEvents([]))
      .finally(() => alive && setLoadingEvents(false))
    return () => {
      alive = false
    }
  }, [expanded, sessionId, typeFilter, refreshKey])

  if (!sum || sum.events === 0) return null

  const topTools = (sum.topTools ?? []).map((name) => ({
    name,
    stat: sum.byTool?.[name],
  }))

  const warnCount = (sum.anomalies ?? []).filter((a) => a.severity === 'warn').length

  // Prompt-cache hit rate across the whole session: the share of prompt tokens served
  // warm (cache read) out of everything paid on the input side (fresh input + cache
  // read + cache write). Mirrors the per-message panel's warm/cold indicator so the
  // session card and the message card agree. '—' when there is no prompt spend yet.
  const promptTotal = sum.inputTokens + sum.cacheReadTokens + sum.cacheWriteTokens
  const cacheHitPct =
    promptTotal > 0 ? `%${Math.round((sum.cacheReadTokens / promptTotal) * 100)}` : '—'

  return (
    <section>
      {/* Foldable header: click to expand/collapse the whole card. Collapsed, it
          still shows a one-glance summary (turns + any warnings). Hidden when
          alwaysOpen (the Debug modal renders its own title). */}
      {!alwaysOpen && (
        <button
          onClick={toggleOpen}
          className="flex w-full items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)] opacity-70 transition hover:opacity-100"
        >
          {open ? (
            <ChevronDown size={12} className="shrink-0" />
          ) : (
            <ChevronRight size={12} className="shrink-0" />
          )}
          <Bug size={12} className="shrink-0" /> {t('debug.title')}
          <InfoPopover text={t('debug.description')} />
          {!open && (
            <span className="ml-auto flex items-center gap-1.5 normal-case tracking-normal">
              <span>{t('debug.turns', { count: sum.turns })}</span>
              {warnCount > 0 && (
                <span className="text-[var(--color-danger)]">
                  · {t('debug.warnings', { count: warnCount })}
                </span>
              )}
            </span>
          )}
        </button>
      )}

      {open && (
        <div className="mt-2">
          {/* Headline metric grid */}
          <div className="grid grid-cols-3 gap-1.5">
            <Metric label={t('debug.metric.turns')} value={String(sum.turns)} />
            <Metric label={t('debug.metric.llmCalls')} value={String(sum.llmCalls)} />
            <Metric label={t('debug.metric.tools')} value={String(sum.toolCalls)} />
            <Metric label={t('debug.metric.inputTokens')} value={fmtTok(sum.inputTokens)} />
            <Metric label={t('debug.metric.outputTokens')} value={fmtTok(sum.outputTokens)} />
            {/* Hidden-reasoning share of output tokens (estimated; already inside
            "Çıkış tok"). Hidden when the session did no measurable thinking. */}
            {(sum.thinkingTokens ?? 0) > 0 && (
              <Metric
                label={t('debug.metric.thinking')}
                value={`%${Math.round((sum.thinkingShare ?? 0) * 100)} · ${fmtTok(sum.thinkingTokens ?? 0)}`}
              />
            )}
            <Metric label={t('debug.metric.cacheRead')} value={fmtTok(sum.cacheReadTokens)} />
            <Metric label={t('debug.metric.cacheWrite')} value={fmtTok(sum.cacheWriteTokens)} />
            <Metric label={t('debug.metric.cacheHit')} value={cacheHitPct} />
          </div>

          {/* Health row: errors / compactions / recoveries */}
          <div className="mt-1.5 flex flex-wrap gap-1.5">
            <Pill
              label={t('debug.health.errors')}
              value={sum.errors}
              tone={sum.errors > 0 ? 'error' : 'dim'}
            />
            <Pill label={t('debug.health.compactions')} value={sum.compactions} tone="dim" />
            <Pill label={t('debug.health.recoveries')} value={sum.recoveries} tone="dim" />
            {sum.cacheBreaks > 0 && (
              <Pill
                label={t('debug.health.cacheBreaks')}
                value={sum.cacheBreaks}
                tone={sum.cacheBreaks >= 2 ? 'error' : 'dim'}
              />
            )}
            {/* Isolated cooling waste: the avoidable overpay from warm prefixes that
            cooled (TTL/eviction) before the next turn — money a timely reply would
            have saved. Estimated ("~") for subscription providers (claude-cli). */}
            {(sum.coolingWasteUsd ?? 0) > 0 && (
              <Pill
                label={t('debug.health.coolingWaste')}
                value={`${sum.coolingWasteEstimated ? '~' : ''}${fmtUsd(sum.coolingWasteUsd ?? 0)}${
                  (sum.coolingBreaks ?? 0) > 0 ? ` · ${sum.coolingBreaks}×` : ''
                }`}
                tone="error"
                raw
              />
            )}
            {sum.turnDurMs > 0 && (
              <Pill
                label={t('debug.health.totalDuration')}
                value={fmtDur(sum.turnDurMs)}
                tone="dim"
                raw
              />
            )}
          </div>

          {/* Anomalies (Faz 3): heuristic findings — slow/failing tools, error
          bursts, frequent compaction. The same notes the reflector learns from. */}
          {sum.anomalies && sum.anomalies.length > 0 && (
            <div className="mt-2 flex flex-col gap-1">
              {sum.anomalies.map((a, i) => {
                const warn = a.severity === 'warn'
                const col = warn ? 'var(--color-danger)' : 'var(--color-text-dim)'
                return (
                  <div
                    key={i}
                    className="flex items-start gap-1.5 rounded-lg border px-2 py-1.5 text-[11px]"
                    style={{
                      color: col,
                      borderColor: `color-mix(in srgb, ${col} 35%, transparent)`,
                      background: `color-mix(in srgb, ${col} 7%, transparent)`,
                    }}
                  >
                    {warn ? (
                      <AlertTriangle size={12} className="mt-0.5 shrink-0" />
                    ) : (
                      <Info size={12} className="mt-0.5 shrink-0" />
                    )}
                    <span className="break-words">{a.message}</span>
                  </div>
                )
              })}
            </div>
          )}

          {/* Time series (Faz 3): per-turn duration + per-call token sparklines. */}
          {((sum.turnDurSeries && sum.turnDurSeries.length > 1) ||
            (sum.tokenSeries && sum.tokenSeries.length > 1)) && (
            <div className="mt-2 grid grid-cols-2 gap-2">
              {sum.turnDurSeries && sum.turnDurSeries.length > 1 && (
                <Sparkline
                  label={t('debug.chart.turnDuration')}
                  data={sum.turnDurSeries}
                  format={fmtDur}
                />
              )}
              {sum.tokenSeries && sum.tokenSeries.length > 1 && (
                <Sparkline
                  label={t('debug.chart.callTokens')}
                  data={sum.tokenSeries}
                  format={fmtTok}
                />
              )}
            </div>
          )}

          {sum.lastError && !sum.anomalies?.some((a) => a.code === 'error_burst') && (
            <div className="mt-1.5 flex items-start gap-1.5 rounded-lg border border-[color-mix(in_srgb,var(--color-danger)_40%,transparent)] bg-[color-mix(in_srgb,var(--color-danger)_8%,transparent)] px-2 py-1.5 text-[11px] text-[var(--color-danger)]">
              <AlertTriangle size={12} className="mt-0.5 shrink-0" />
              <span className="break-words">{sum.lastError}</span>
            </div>
          )}

          {/* Slowest tools (the optimisation hot list) */}
          {topTools.length > 0 && (
            <div className="mt-2">
              <div className="mb-1 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
                {t('debug.slowestTools')}
              </div>
              <div className="flex flex-col gap-1">
                {topTools.map(({ name, stat }) => (
                  <div key={name} className="flex items-center justify-between text-[11px]">
                    <span
                      className="truncate text-[var(--color-text-dim)]"
                      title={toolDisplayName(name)}
                    >
                      {toolDisplayName(name)}
                      {stat && stat.errors > 0 && (
                        <span className="ml-1 text-[var(--color-danger)]">
                          ·{t('debug.errors', { count: stat.errors })}
                        </span>
                      )}
                    </span>
                    <span className="ml-2 shrink-0 text-[var(--color-text)]">
                      {stat ? `${fmtDur(stat.durMs)} · ${stat.calls}×` : '—'}
                    </span>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Token spend by model */}
          {sum.byModel && Object.keys(sum.byModel).length > 0 && (
            <div className="mt-2">
              <div className="mb-1 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
                {t('debug.tokensByModel')}
              </div>
              <div className="flex flex-col gap-1">
                {Object.entries(sum.byModel)
                  .sort((a, b) => b[1] - a[1])
                  .slice(0, 4)
                  .map(([model, tok]) => (
                    <div key={model} className="flex items-center justify-between text-[11px]">
                      <span className="truncate text-[var(--color-text-dim)]" title={model}>
                        {modelDisplayName(model)}
                      </span>
                      <span className="ml-2 shrink-0 text-[var(--color-text)]">{fmtTok(tok)}</span>
                    </div>
                  ))}
              </div>
            </div>
          )}

          {/* Workflow visualizations: tool-execution Sankey + concurrency timeline
          (foldable, lazily fetches its own raw events). */}
          <SessionFlowViz sessionId={sessionId} agentNames={agentNames} refreshKey={refreshKey} />

          {/* Raw event log (lazy) */}
          <button
            onClick={() => setExpanded((v) => !v)}
            className="mt-2 flex items-center gap-1 text-[11px] text-[var(--color-accent)] underline-offset-2 hover:underline"
          >
            {expanded ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
            {t('debug.rawEvents', { count: sum.events })}
          </button>
          {expanded && (
            <div className="mt-1.5">
              <div className="mb-1.5 flex flex-wrap gap-1">
                {[
                  '',
                  'turn',
                  'llm_call',
                  'tool',
                  'hook',
                  'error',
                  'compaction',
                  'recovery',
                  'cache_break',
                  'repair',
                  'guardrail',
                  'lesson',
                  'epoch',
                  'pressure',
                  'build',
                  'cli_compaction',
                  'lifecycle',
                  'thinking_dropped',
                ].map((t) => (
                  <button
                    key={t || 'all'}
                    onClick={() => setTypeFilter(t)}
                    className={`rounded px-1.5 py-0.5 text-[10px] transition ${
                      typeFilter === t
                        ? 'bg-[var(--color-accent)] text-[var(--color-on-accent)]'
                        : 'border border-[var(--color-border)] text-[var(--color-text-dim)] hover:text-[var(--color-text)]'
                    }`}
                  >
                    {t || i18next.t('debug.all', { ns: 'sessions' })}
                  </button>
                ))}
              </div>
              {loadingEvents ? (
                <div className="flex items-center gap-1.5 py-2 text-[11px] text-[var(--color-text-dim)]">
                  <Loader2 size={12} className="animate-spin" /> {t('common.loading')}
                </div>
              ) : events && events.length > 0 ? (
                <div className="max-h-64 overflow-y-auto rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] font-mono text-[10px]">
                  {events
                    .slice()
                    .reverse()
                    .map((e, i) => (
                      <div
                        key={i}
                        className={`flex items-center gap-1.5 border-b border-[var(--color-border)] px-2 py-1 last:border-b-0 ${
                          e.err ? 'text-[var(--color-danger)]' : 'text-[var(--color-text-dim)]'
                        }`}
                      >
                        <span className="w-16 shrink-0 opacity-60">{fmtTime(e.ts)}</span>
                        <span className="w-20 shrink-0 font-semibold text-[var(--color-text)]">
                          {e.type}
                        </span>
                        <span className="min-w-0 flex-1 truncate">{eventLabel(e)}</span>
                      </div>
                    ))}
                </div>
              ) : (
                <p className="py-2 text-[11px] text-[var(--color-text-dim)]">
                  {t('debug.noEvents')}
                </p>
              )}
            </div>
          )}
        </div>
      )}
    </section>
  )
}

// Sparkline draws a tiny dependency-free SVG line chart of a numeric series,
// with the latest value labelled. Used for the per-turn duration and per-call
// token trends in the debug card.
function Sparkline({
  label,
  data,
  format,
}: {
  label: string
  data: number[]
  format: (n: number) => string
}) {
  const w = 120
  const h = 28
  const max = Math.max(...data, 1)
  const min = Math.min(...data, 0)
  const span = max - min || 1
  const step = data.length > 1 ? w / (data.length - 1) : w
  const pts = data
    .map((v, i) => {
      const x = i * step
      const y = h - ((v - min) / span) * (h - 4) - 2
      return `${x.toFixed(1)},${y.toFixed(1)}`
    })
    .join(' ')
  const last = data[data.length - 1]
  return (
    <div className="rounded-lg border border-[var(--color-border)] px-2 py-1.5">
      <div className="mb-0.5 flex items-center justify-between">
        <span className="text-[9px] uppercase tracking-wide text-[var(--color-text-dim)]">
          {label}
        </span>
        <span className="text-[10px] font-semibold text-[var(--color-text)]">{format(last)}</span>
      </div>
      <svg viewBox={`0 0 ${w} ${h}`} width="100%" height={h} preserveAspectRatio="none">
        <polyline
          points={pts}
          fill="none"
          stroke="var(--color-accent)"
          strokeWidth={1.5}
          strokeLinejoin="round"
          strokeLinecap="round"
        />
      </svg>
    </div>
  )
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg border border-[var(--color-border)] px-2 py-1.5">
      <div className="text-[9px] uppercase tracking-wide text-[var(--color-text-dim)]">{label}</div>
      <div className="text-sm font-semibold text-[var(--color-text)]">{value}</div>
    </div>
  )
}

function Pill({
  label,
  value,
  tone,
  raw,
}: {
  label: string
  value: number | string
  tone: 'error' | 'dim'
  raw?: boolean
}) {
  const color = tone === 'error' ? 'var(--color-danger)' : 'var(--color-text-dim)'
  return (
    <span
      className="rounded-md border border-[var(--color-border)] px-1.5 py-0.5 text-[10px]"
      style={{ color }}
    >
      {label}: <b>{raw ? value : value}</b>
    </span>
  )
}

// eventLabel renders the type-specific one-line detail for a raw event row.
function eventLabel(e: SessionDebugEvent): string {
  switch (e.type) {
    case 'turn':
      return `${fmtDur(e.durMs ?? 0)}${e.stop ? ` · ${e.stop}` : ''}${e.err ? ` · ${i18next.t('debug.event.error', { ns: 'sessions' })}` : ''}`
    case 'llm_call':
      return `${e.model || i18next.t('debug.event.model', { ns: 'sessions' })} · ${i18next.t('debug.event.input', { ns: 'sessions' })} ${e.in ?? 0} / ${i18next.t('debug.event.output', { ns: 'sessions' })} ${e.out ?? 0}${
        e.cacheRead ? ` / cache ${e.cacheRead}` : ''
      }`
    case 'tool':
      return `${e.name} · ${fmtDur(e.durMs ?? 0)} · ${fmtBytes(e.outBytes ?? 0)}${
        e.err ? ` · ${i18next.t('debug.event.error', { ns: 'sessions' })}` : ''
      }`
    case 'hook':
      return `${e.name} · ${e.detail ?? ''}`
    case 'compaction':
      return `${e.detail ?? ''}${e.savedBytes ? ` · ${fmtBytes(e.savedBytes)}` : ''}`
    case 'recovery':
      return e.detail ?? ''
    case 'cache_break':
      return `${e.name ?? 'cache-break'}${e.detail ? ` · ${e.detail}` : ''}${
        e.cacheWrite
          ? ` · ${i18next.t('debug.event.rewritten', { ns: 'sessions', count: e.cacheWrite })}`
          : ''
      }${e.wasteUsd ? ` · ${i18next.t('debug.event.waste', { ns: 'sessions' })} ${e.wasteEst ? '~' : ''}${fmtUsd(e.wasteUsd)}` : ''}`
    case 'epoch':
      return `${e.name ?? 'epoch'}${e.detail ? ` · ${e.detail}` : ''}`
    case 'repair':
      return `${e.name ?? 'repair'}${e.detail ? ` · ${e.detail}` : ''}`
    case 'guardrail':
      return `${e.name ?? 'guardrail'}${e.detail ? ` · ${e.detail}` : ''}`
    case 'lesson':
      return `${e.name ?? 'lesson'}${e.detail ? ` · ${e.detail}` : ''}`
    case 'pressure':
      return `${e.name ?? 'pressure'}${e.detail ? ` · ${e.detail}` : ''}`
    case 'build':
      return `${e.name ?? '(commit)'}${e.detail ? ` · ${e.detail}` : ''}`
    case 'cli_compaction':
      return `${e.name ?? 'cli-compaction'}${e.detail ? ` · ${e.detail}` : ''}${
        e.durMs ? ` · ${fmtDur(e.durMs)}` : ''
      }`
    case 'lifecycle':
      return `${e.name ?? 'lifecycle'}${e.detail ? ` · ${e.detail}` : ''}`
    case 'thinking_dropped':
      return `${i18next.t('debug.event.thinkingDropped', { ns: 'sessions', count: e.calls ?? 1 })} · ${e.name ?? ''}${e.detail ? ` · ${e.detail}` : ''}`
    case 'error':
      return e.detail ?? ''
    default:
      return e.detail ?? ''
  }
}

function fmtTime(ms: number): string {
  try {
    const d = new Date(ms)
    return formatTime(d, { hour12: false })
  } catch {
    return ''
  }
}
