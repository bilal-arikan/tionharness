import { useEffect, useMemo, useState } from 'react'
import {
  RefreshCw,
  Coins,
  Hash,
  ArrowDownToLine,
  DollarSign,
  ChevronRight,
  ChevronDown,
  PiggyBank,
  Percent,
  Sigma,
  Snowflake,
} from 'lucide-react'
import { api } from '@/api'
import { PaneHeader } from '@/shared/components'
import type { KindStat, ProviderStat, BudgetTrendPoint } from '@/types'
import { AgentAvatar } from '@/shared/components/agents/AgentAvatar'
import { kindColor } from '@/shared/lib/palette'
import { percent, tokens as fmt, usd } from '@/shared/lib/format'
import { formatDate } from '@/shared/lib/intl'
import { i18next } from '@/i18n'
import { modelDisplayName } from '@/shared/lib/modelLabel'
import { useAsync } from '@/shared/hooks/useAsync'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { DecisionSpendSection } from './DecisionSpendSection'

interface Props {
  onError: (msg: string) => void
}

// TrendMetric selects which series the daily trend chart plots. Each maps a
// BudgetTrendPoint to a scalar, a formatter, a bar colour and a tooltip suffix
// so the same chart can show token volume, spend or caching ROI.
type TrendMetric = 'token' | 'cost' | 'cacheSave'

interface TrendMetricDef {
  key: TrendMetric
  label: string
  value: (p: BudgetTrendPoint) => number
  fmt: (n: number) => string
  color: string
}

function trendMetrics(t: TFunction<'budget'>): TrendMetricDef[] {
  return [
    {
      key: 'token',
      label: t('trend.metric.token'),
      value: (p) => p.inputTokens + p.outputTokens,
      fmt,
      color: 'var(--color-accent)',
    },
    {
      key: 'cost',
      label: t('trend.metric.cost'),
      value: (p) => p.costUSD,
      fmt: usd,
      color: 'var(--color-warning)',
    },
    {
      key: 'cacheSave',
      label: t('trend.metric.cacheSavings'),
      value: (p) => p.savingsUSD,
      fmt: usd,
      color: 'var(--color-success)',
    },
  ]
}

// Provider display labels; claude-cli is a flat subscription, not metered.
const PROVIDER_LABEL: Record<string, string> = {
  anthropic: 'Anthropic',
  'claude-cli': 'Claude CLI',
  minimax: 'MiniMax',
  'minimax-anthropic': 'MiniMax (Anthropic)',
}

function providerLabel(p: string): string {
  return PROVIDER_LABEL[p] ?? p
}

function kindMeta(kind: string, t: TFunction<'budget'>) {
  const labels: Record<string, string> = {
    chat: t('kind.chat'),
    task: t('kind.task'),
    schedule: t('kind.schedule'),
    flow: t('kind.flow'),
    delegate: t('kind.delegate'),
    title: t('kind.title'),
    summary: t('kind.summary'),
    reflect: t('kind.reflect'),
    compact: t('kind.compact'),
    decide: t('kind.decide'),
    system: t('kind.system'),
    other: t('kind.other'),
  }
  const [prefix, operation] = kind.split(':', 2)
  const label = operation
    ? `${labels[prefix] ?? prefix} · ${labels[operation] ?? operation}`
    : (labels[kind] ?? kind)
  return { label, color: kindColor(kind) }
}

function tokensOf(s: KindStat): number {
  return s.inputTokens + s.outputTokens
}

// costText renders a cost cell honoring the priced/estimated flags.
// Subscription providers (claude-cli) have an estimated equivalent-API cost
// shown with a "~" prefix and a tooltip explaining it is not real billing.
function costText(
  costUSD: number,
  priced: boolean,
  estimated: boolean | undefined,
  t: TFunction<'budget'>,
): React.ReactNode {
  if (priced) return usd(costUSD)
  if (costUSD > 0) {
    const title = estimated ? t('cost.subscriptionEstimateHint') : t('cost.partlyUnpricedHint')
    return <span title={title}>~{usd(costUSD)}</span>
  }
  return <span className="text-[var(--color-text-dim)]">{t('cost.subscriptionOrUnpriced')}</span>
}

// cacheText renders the "read/write" cache token pair, dimmed when zero.
function cacheText(read: number, write: number): React.ReactNode {
  if (read === 0 && write === 0)
    return (
      <span className="text-[var(--color-text-dim)]">
        {i18next.t('notAvailable', { ns: 'budget' })}
      </span>
    )
  return (
    <span className="text-[var(--color-text-dim)]">
      {fmt(read)}/{fmt(write)}
    </span>
  )
}

// FragmentRows renders a provider summary row plus, when expanded, one detail
// row per model under it. A provider with a single model still expands so the
// model id is visible.
function FragmentRows({
  open,
  onToggle,
  provider: p,
}: {
  open: boolean
  onToggle: () => void
  provider: ProviderStat
}) {
  const { t } = useTranslation('budget')
  return (
    <>
      <tr
        className="cursor-pointer border-t border-[var(--color-border)] hover:bg-[var(--color-surface)]"
        onClick={onToggle}
      >
        <td className="px-4 py-2.5 text-[var(--color-text)]">
          <span className="flex items-center gap-1.5">
            {p.models.length > 0 ? (
              open ? (
                <ChevronDown size={13} className="text-[var(--color-text-dim)]" />
              ) : (
                <ChevronRight size={13} className="text-[var(--color-text-dim)]" />
              )
            ) : (
              <span className="w-[13px]" />
            )}
            {providerLabel(p.provider)}
          </span>
        </td>
        <td className="px-4 py-2.5 text-[var(--color-text-dim)]">{fmt(p.calls)}</td>
        <td className="px-4 py-2.5 text-[var(--color-text-dim)]">
          {fmt(p.inputTokens + p.outputTokens)}{' '}
          <span className="opacity-60">
            ({fmt(p.inputTokens)}/{fmt(p.outputTokens)})
          </span>
        </td>
        <td className="px-4 py-2.5">{cacheText(p.cacheReadTokens, p.cacheWriteTokens)}</td>
        <td className="px-4 py-2.5 text-[var(--color-text-dim)]">
          {p.savingsUSD > 0 ? (
            <span style={{ color: 'var(--color-success)' }}>{usd(p.savingsUSD)}</span>
          ) : (
            '—'
          )}
        </td>
        <td className="px-4 py-2.5 text-[var(--color-text)]">
          {costText(p.costUSD, p.priced, p.estimated, t)}
        </td>
      </tr>
      {open &&
        p.models.map((m) => (
          <tr
            key={m.model}
            className="border-t border-[var(--color-border)] bg-[var(--color-surface)]"
          >
            <td
              className="py-2 pl-11 pr-4 text-xs text-[var(--color-text-dim)]"
              title={m.model || undefined}
            >
              {modelDisplayName(m.model)}
            </td>
            <td className="px-4 py-2 text-xs text-[var(--color-text-dim)]">{fmt(m.calls)}</td>
            <td className="px-4 py-2 text-xs text-[var(--color-text-dim)]">
              {fmt(m.inputTokens + m.outputTokens)}{' '}
              <span className="opacity-60">
                ({fmt(m.inputTokens)}/{fmt(m.outputTokens)})
              </span>
            </td>
            <td className="px-4 py-2 text-xs">
              {cacheText(m.cacheReadTokens, m.cacheWriteTokens)}
            </td>
            <td className="px-4 py-2 text-xs text-[var(--color-text-dim)]">
              {m.savingsUSD > 0 ? (
                <span style={{ color: 'var(--color-success)' }}>{usd(m.savingsUSD)}</span>
              ) : (
                '—'
              )}
            </td>
            <td className="px-4 py-2 text-xs text-[var(--color-text)]">
              {costText(m.costUSD, m.priced, m.estimated, t)}
            </td>
          </tr>
        ))}
    </>
  )
}

// SummaryCard is one headline metric at the top of the screen.
function SummaryCard({
  icon,
  label,
  value,
  sub,
}: {
  icon: React.ReactNode
  label: string
  value: string
  sub?: string
}) {
  return (
    <div className="flex-1 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] p-4">
      <div className="flex items-center gap-2 text-xs text-[var(--color-text-dim)]">
        {icon}
        {label}
      </div>
      <div className="mt-1 text-2xl font-semibold text-[var(--color-text)]">{value}</div>
      {sub && <div className="text-xs text-[var(--color-text-dim)]">{sub}</div>}
    </div>
  )
}

// SavingsCell is one optimization source's contribution inside the Tasarruf
// Merkezi grid: a title, a prominent value, a sub-line, and a tooltip explaining
// what the figure means and whether it is real billing or an estimate.
function SavingsCell({
  title,
  primary,
  sub,
  hint,
  // 'success' (green) for real savings; 'warning' (orange) for a leak like
  // cooling waste — a negative value, so painting it green would misread as a gain.
  tone = 'success',
}: {
  title: string
  primary: string
  sub: string
  hint: string
  tone?: 'success' | 'warning'
}) {
  return (
    <div className="px-4 py-3" title={hint}>
      <div className="text-[11px] text-[var(--color-text-dim)]">{title}</div>
      <div className="mt-0.5 text-xl font-semibold" style={{ color: `var(--color-${tone})` }}>
        {primary}
      </div>
      <div className="mt-0.5 text-[11px] text-[var(--color-text-dim)]">{sub}</div>
    </div>
  )
}

// BudgetPanel is the workspace-wide budget/usage screen: today's totals, a
// per-origin breakdown (the new ByKind data), a daily trend, and a per-agent
// spend table with limit fill bars.
export function BudgetPanel({ onError }: Props) {
  const { t } = useTranslation('budget')
  const [days, setDays] = useState(7)
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  // Which series the daily trend chart plots. Token volume by default; the other
  // metrics let the same window be read as spend, caching ROI, or compaction.
  const [trendMetric, setTrendMetric] = useState<TrendMetric>('token')

  const toggleProvider = (p: string) =>
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(p)) next.delete(p)
      else next.add(p)
      return next
    })

  // Fetch workspace usage for the selected window; re-runs when `days` changes
  // and can be re-triggered by the refresh button. Errors surface via onError.
  const {
    data: usage,
    loading,
    error,
    refresh: load,
  } = useAsync(() => api.workspaceUsage(days), [days])
  useEffect(() => {
    if (error) onError(error)
  }, [error, onError])

  const totalTokens = usage ? usage.totals.inputTokens + usage.totals.outputTokens : 0

  // Origins sorted by token spend, biggest first. Any spend not attributed to a
  // kind — e.g. usage recorded before origin-tagging shipped — is folded into a
  // synthetic "other" bucket so the breakdown still reflects the full total
  // instead of looking empty when there clearly was spend.
  const kinds = useMemo(() => {
    if (!usage) return []
    const list = Object.entries(usage.totals.byKind).map(([kind, st]) => ({
      kind,
      st,
      tokens: tokensOf(st),
    }))
    const tagged = list.reduce((a, k) => a + k.tokens, 0)
    const taggedCalls = list.reduce((a, k) => a + k.st.calls, 0)
    const remTokens = totalTokens - tagged
    const remCalls = usage.totals.calls - taggedCalls
    if (remTokens > 0 || (totalTokens === 0 && usage.totals.calls > remCalls)) {
      const existing = list.find((k) => k.kind === 'other')
      if (existing) {
        existing.tokens += remTokens
        existing.st = { ...existing.st, calls: existing.st.calls + remCalls }
      } else {
        list.push({
          kind: 'other',
          st: { calls: Math.max(0, remCalls), inputTokens: 0, outputTokens: 0 },
          tokens: Math.max(0, remTokens),
        })
      }
    }
    return list.sort((a, b) => b.tokens - a.tokens)
  }, [usage, totalTokens])

  const metrics = trendMetrics(t)
  const metricDef = metrics.find((m) => m.key === trendMetric) ?? metrics[0]

  const trendMax = usage ? Math.max(1e-9, ...usage.trend.map((point) => metricDef.value(point))) : 0

  return (
    <div className="flex h-full flex-col bg-[var(--color-surface)]">
      <PaneHeader
        title={t('title')}
        subtitle={
          usage
            ? `· ${formatDate(new Date(`${usage.day}T00:00:00Z`), {
                dateStyle: 'medium',
                timeZone: 'UTC',
              })}`
            : undefined
        }
        right={
          <>
            <div className="flex overflow-hidden rounded border border-[var(--color-border)] text-xs">
              {[7, 30, 90].map((d) => (
                <button
                  key={d}
                  onClick={() => setDays(d)}
                  className={`px-2 py-1 transition ${
                    days === d
                      ? 'bg-[var(--color-accent)] text-[var(--color-on-accent)]'
                      : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)] hover:opacity-80'
                  }`}
                >
                  {t('rangeDays', { count: d })}
                </button>
              ))}
            </div>
            <button
              onClick={() => load()}
              className="rounded border border-[var(--color-border)] bg-[var(--color-surface-2)] p-1.5 text-[var(--color-text-dim)] transition hover:opacity-80"
              title={t('refresh')}
            >
              <RefreshCw size={14} className={loading ? 'animate-spin' : ''} />
            </button>
          </>
        }
      />
      <div className="flex-1 overflow-y-auto p-3 md:p-5">
        {!usage ? (
          <div className="text-sm text-[var(--color-text-dim)]">
            {loading ? t('loading') : t('noData')}
          </div>
        ) : (
          <>
            {usage.decisionSpend && <DecisionSpendSection report={usage.decisionSpend} />}
            {/* Summary cards — today */}
            <div className="mb-5 grid grid-cols-2 gap-3 lg:grid-cols-4">
              <SummaryCard
                icon={<Coins size={12} />}
                label={t('summary.tokensToday')}
                value={fmt(totalTokens)}
                sub={t('summary.inputOutput', {
                  input: fmt(usage.totals.inputTokens),
                  output: fmt(usage.totals.outputTokens),
                })}
              />
              <SummaryCard
                icon={<Hash size={12} />}
                label={t('summary.callsToday')}
                value={fmt(usage.totals.calls)}
              />
              <SummaryCard
                icon={<ArrowDownToLine size={12} />}
                label={t('summary.inputTokens')}
                value={fmt(usage.totals.inputTokens)}
              />
              <SummaryCard
                icon={<DollarSign size={12} />}
                label={t('summary.estimatedCostToday')}
                value={`${usage.totals.priced ? '' : '~'}${usd(usage.totals.costUSD)}`}
                sub={
                  usage.totals.priced
                    ? t('summary.listPriceEstimate')
                    : usage.totals.estimated
                      ? t('summary.cliEquivalentIncluded')
                      : t('summary.partlySubscription')
                }
              />
            </div>

            {/* General claude-cli overhead note: when a subscription CLI provider is
              in use (estimated cost), remind that the real billed input dwarfs the
              context-preview segment estimate (the CLI injects its own prompt +
              tools + MCP bridge), and that THESE Budget figures are the real spend.
              The eager-vs-lazy tool split is called out here too: the "info screen
              counts N tools but the context popup shows few" gap is the same effect —
              only eager schemas ship each turn; deferred (lazy) MCP + self-management
              tools activated mid-session via ToolSearch stay warm in --resume and
              inflate the real billed input without appearing in the popup's eager
              "Araçlar" count. See the session context popup's "Talep-üzerine" chip. */}
            {usage.totals.estimated && (
              <div className="mb-5 flex items-start gap-2 rounded-lg border border-[color-mix(in_srgb,var(--color-warning,#d97706)_30%,transparent)] bg-[color-mix(in_srgb,var(--color-warning,#d97706)_8%,transparent)] px-3 py-2 text-[11px] text-[var(--color-text-dim)]">
                <span className="text-[var(--color-warning,#d97706)]">{t('infoSymbol')}</span>
                <span>
                  <strong className="text-[var(--color-text)]">{t('cliOverhead.title')}</strong>{' '}
                  {t('cliOverhead.descriptionBefore')} <strong>{t('cliOverhead.billed')}</strong>{' '}
                  {t('cliOverhead.descriptionAfter')}
                  <br />
                  <span className="mt-1 inline-block">
                    <strong className="text-[var(--color-text)]">
                      {t('cliOverhead.toolCountTitle')}
                    </strong>{' '}
                    {t('cliOverhead.toolCountBefore')} <em>{t('cliOverhead.toolsLabel')}</em>{' '}
                    {t('cliOverhead.toolCountMiddle')} <em>{t('cliOverhead.eager')}</em>{' '}
                    {t('cliOverhead.toolCountSet')}{' '}
                    <code className="rounded bg-[var(--color-surface-2)] px-1">ToolSearch</code>{' '}
                    {t('cliOverhead.activatedWith')} <em>{t('cliOverhead.deferred')}</em>{' '}
                    {t('cliOverhead.tools')}{' '}
                    <code className="rounded bg-[var(--color-surface-2)] px-1">--resume</code>{' '}
                    {t('cliOverhead.resumeAfter')} <em>{t('cliOverhead.onDemandLabel')}</em>{' '}
                    {t('cliOverhead.chip')}
                  </span>
                </span>
              </div>
            )}

            {/* Window-cumulative ROI — cross-session totals over the selected
              window (today's cards above are just one day). The cache hit rate
              and total savings are the caching ROI signal. */}
            <div className="mb-5 grid grid-cols-2 gap-3 lg:grid-cols-4">
              <SummaryCard
                icon={<Sigma size={12} />}
                label={t('cumulative.totalCost', { count: days })}
                value={`${usage.cumulative.costUSD > 0 && !usage.totals.priced ? '~' : ''}${usd(usage.cumulative.costUSD)}`}
                sub={t('cumulative.callsTokens', {
                  calls: fmt(usage.cumulative.calls),
                  tokens: fmt(usage.cumulative.inputTokens + usage.cumulative.outputTokens),
                })}
              />
              <SummaryCard
                icon={<PiggyBank size={12} />}
                label={t('cumulative.cacheSavings', { count: days })}
                value={usd(usage.cumulative.savingsUSD)}
                sub={t('cumulative.cacheReadWrite', {
                  read: fmt(usage.cumulative.cacheReadTokens),
                  write: fmt(usage.cumulative.cacheWriteTokens),
                })}
              />
              <SummaryCard
                icon={<Percent size={12} />}
                label={t('cumulative.cacheHitRate')}
                value={percent(usage.cumulative.cacheHitRate)}
                sub={t('cumulative.cacheHitRateHint')}
              />
              <SummaryCard
                icon={<DollarSign size={12} />}
                label={t('cumulative.noCacheCost')}
                value={`${usage.cumulative.noCacheCostUSD > 0 && !usage.totals.priced ? '~' : ''}${usd(usage.cumulative.noCacheCostUSD)}`}
                sub={t('cumulative.noCacheCostHint')}
              />
              {/* Cooling waste: avoidable overpay from warm prefixes that cooled
                (TTL/eviction) before the next turn. Only shown when it occurred —
                a leak, not a saving, so it reads as an alert. "~" when estimated. */}
              {(usage.cumulative.coolingWasteUSD ?? 0) > 0 && (
                <SummaryCard
                  icon={<Snowflake size={12} />}
                  label={t('cumulative.coolingWaste', { count: days })}
                  value={`${usage.cumulative.coolingWasteEstimated ? '~' : ''}${usd(usage.cumulative.coolingWasteUSD ?? 0)}`}
                  sub={t('cumulative.coolingWasteHint')}
                />
              )}
            </div>

            {/* Tasarruf Merkezi — prompt-cache USD savings, the only source with
              real billing impact. */}
            <div className="mb-5 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)]">
              <div className="flex items-center gap-2 border-b border-[var(--color-border)] px-4 py-2.5">
                <PiggyBank size={15} style={{ color: 'var(--color-success)' }} />
                <span className="text-sm font-medium text-[var(--color-text)]">
                  {t('savings.title')}
                </span>
                <span className="text-xs text-[var(--color-text-dim)]">
                  {t('savings.window', { count: days })}
                </span>
              </div>
              <div className="grid grid-cols-1">
                {/* Prompt-cache — the only source with real USD billing impact. */}
                <SavingsCell
                  title={t('savings.promptCache')}
                  primary={usd(usage.cumulative.savingsUSD)}
                  sub={t('savings.promptCacheSub', {
                    tokens: fmt(usage.cumulative.cacheReadTokens),
                    rate: percent(usage.cumulative.cacheHitRate),
                  })}
                  hint={t('savings.promptCacheHint')}
                />
                {/* Cooling waste — the inverse of the saving above: warm prefixes lost
                  to TTL/eviction before the next turn, re-written at the write tier.
                  Only rendered when it happened, as a recoverable leak. */}
                {(usage.cumulative.coolingWasteUSD ?? 0) > 0 && (
                  <div className="border-t border-[var(--color-border)]">
                    <SavingsCell
                      tone="warning"
                      title={t('savings.coolingWaste')}
                      primary={`${usage.cumulative.coolingWasteEstimated ? '~' : ''}−${usd(usage.cumulative.coolingWasteUSD ?? 0)}`}
                      sub={t('savings.coolingWasteSub')}
                      hint={t('savings.coolingWasteHint')}
                    />
                  </div>
                )}
              </div>
            </div>

            <div className="mb-5 grid grid-cols-1 gap-4 lg:grid-cols-2">
              {/* Origin breakdown */}
              <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] p-4">
                <div className="mb-3 text-sm font-medium text-[var(--color-text)]">
                  {t('origin.title')}
                </div>
                {kinds.length === 0 ? (
                  <div className="text-xs text-[var(--color-text-dim)]">{t('origin.empty')}</div>
                ) : (
                  <div className="space-y-2">
                    {kinds.map(({ kind, st, tokens }) => {
                      const pct = totalTokens > 0 ? (tokens / totalTokens) * 100 : 0
                      const m = kindMeta(kind, t)
                      return (
                        <div key={kind}>
                          <div className="mb-0.5 flex items-center justify-between text-xs">
                            <span className="flex items-center gap-1.5 text-[var(--color-text)]">
                              <span
                                className="h-2.5 w-2.5 rounded-sm"
                                style={{ background: m.color }}
                              />
                              {m.label}
                            </span>
                            <span className="text-[var(--color-text-dim)]">
                              {t('origin.rowMeta', {
                                tokens: fmt(tokens),
                                percent: percent(pct / 100),
                                calls: fmt(st.calls),
                              })}
                            </span>
                          </div>
                          <div className="h-1.5 w-full overflow-hidden rounded-full bg-[var(--color-surface)]">
                            <div
                              className="h-full rounded-full"
                              style={{ width: `${pct}%`, background: m.color }}
                            />
                          </div>
                        </div>
                      )
                    })}
                  </div>
                )}
              </div>

              {/* Trend */}
              <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] p-4">
                <div className="mb-3 flex items-center justify-between gap-2">
                  <div className="text-sm font-medium text-[var(--color-text)]">
                    {t('trend.title', { count: days })}
                  </div>
                  {/* Metric selector: plot the same window as token volume, spend
                    or caching ROI. */}
                  <div className="flex overflow-hidden rounded border border-[var(--color-border)] text-[11px]">
                    {metrics.map((m) => (
                      <button
                        key={m.key}
                        onClick={() => setTrendMetric(m.key)}
                        className={`px-2 py-1 transition ${
                          trendMetric === m.key
                            ? 'bg-[var(--color-accent)] text-[var(--color-on-accent)]'
                            : 'bg-[var(--color-surface)] text-[var(--color-text-dim)] hover:opacity-80'
                        }`}
                      >
                        {m.label}
                      </button>
                    ))}
                  </div>
                </div>
                {usage.trend.length === 0 ? (
                  <div className="text-xs text-[var(--color-text-dim)]">{t('trend.empty')}</div>
                ) : (
                  <div className="flex h-32 items-end gap-1">
                    {usage.trend.map((p) => {
                      const v = metricDef.value(p)
                      const h = (v / trendMax) * 100
                      const tok = p.inputTokens + p.outputTokens
                      return (
                        <div
                          key={p.day}
                          className="flex-1 rounded-t transition-all hover:opacity-80"
                          style={{ height: `${Math.max(2, h)}%`, background: metricDef.color }}
                          title={t('trend.tooltip', {
                            day: formatDate(new Date(`${p.day}T00:00:00Z`), {
                              dateStyle: 'medium',
                              timeZone: 'UTC',
                            }),
                            metric: metricDef.label,
                            value: metricDef.fmt(v),
                            tokens: fmt(tok),
                            calls: fmt(p.calls),
                            cost: usd(p.costUSD),
                            savings:
                              p.savingsUSD > 0
                                ? t('trend.tooltipSavings', { value: usd(p.savingsUSD) })
                                : '',
                          })}
                        />
                      )
                    })}
                  </div>
                )}
                <div className="mt-2 text-[11px] text-[var(--color-text-dim)]">
                  {t('trend.total', {
                    metric: metricDef.label,
                    value: metricDef.fmt(usage.trend.reduce((a, p) => a + metricDef.value(p), 0)),
                  })}
                </div>
              </div>
            </div>

            {/* Per-provider breakdown (expandable to per-model detail) */}
            <div className="mb-5 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)]">
              <div className="flex items-center justify-between border-b border-[var(--color-border)] px-4 py-2.5">
                <span className="text-sm font-medium text-[var(--color-text)]">
                  {t('provider.title')}
                </span>
                {usage.totals.savingsUSD > 0 && (
                  <span
                    className="rounded px-2 py-0.5 text-xs"
                    style={{
                      background: 'color-mix(in srgb, var(--color-success) 15%, transparent)',
                      color: 'var(--color-success)',
                    }}
                    title={t('provider.cacheSavingsHint')}
                  >
                    {t('provider.cacheSavings', { value: usd(usage.totals.savingsUSD) })}
                  </span>
                )}
              </div>
              {usage.byProvider.length === 0 ? (
                <div className="px-4 py-3 text-xs text-[var(--color-text-dim)]">
                  {t('provider.empty')}
                </div>
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full min-w-[36rem] text-sm">
                    <thead>
                      <tr className="text-left text-xs text-[var(--color-text-dim)]">
                        <th className="px-4 py-2 font-medium">
                          {t('provider.columns.providerModel')}
                        </th>
                        <th className="px-4 py-2 font-medium">{t('provider.columns.calls')}</th>
                        <th className="px-4 py-2 font-medium">{t('provider.columns.tokens')}</th>
                        <th className="px-4 py-2 font-medium">{t('provider.columns.cache')}</th>
                        <th className="px-4 py-2 font-medium">{t('provider.columns.savings')}</th>
                        <th className="px-4 py-2 font-medium">{t('provider.columns.cost')}</th>
                      </tr>
                    </thead>
                    <tbody>
                      {usage.byProvider.map((p) => {
                        const open = expanded.has(p.provider)
                        return (
                          <FragmentRows
                            key={p.provider}
                            open={open}
                            onToggle={() => toggleProvider(p.provider)}
                            provider={p}
                          />
                        )
                      })}
                    </tbody>
                  </table>
                </div>
              )}
            </div>

            {/* Per-agent table */}
            <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)]">
              <div className="border-b border-[var(--color-border)] px-4 py-2.5 text-sm font-medium text-[var(--color-text)]">
                {t('agents.title')}
              </div>
              {usage.agents.length === 0 ? (
                <div className="px-4 py-3 text-xs text-[var(--color-text-dim)]">
                  {t('agents.empty')}
                </div>
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full min-w-[28rem] text-sm">
                    <thead>
                      <tr className="text-left text-xs text-[var(--color-text-dim)]">
                        <th className="px-4 py-2 font-medium">{t('agents.columns.agent')}</th>
                        <th className="px-4 py-2 font-medium">{t('agents.columns.calls')}</th>
                        <th className="px-4 py-2 font-medium">{t('agents.columns.tokens')}</th>
                        <th className="px-4 py-2 font-medium">{t('agents.columns.cost')}</th>
                      </tr>
                    </thead>
                    <tbody>
                      {usage.agents.map((a) => {
                        const tok = a.inputTokens + a.outputTokens
                        return (
                          <tr key={a.agentId} className="border-t border-[var(--color-border)]">
                            <td className="px-4 py-2.5">
                              <div className="flex items-center gap-2">
                                <AgentAvatar
                                  agent={{
                                    id: a.agentId,
                                    name: a.name,
                                    avatar: a.avatar,
                                    color: a.color,
                                  }}
                                  size={22}
                                />
                                <span className="text-[var(--color-text)]">
                                  {a.name || t('agents.unnamed')}
                                </span>
                              </div>
                            </td>
                            <td className="px-4 py-2.5 text-[var(--color-text-dim)]">
                              {fmt(a.calls)}
                            </td>
                            <td className="px-4 py-2.5 text-[var(--color-text-dim)]">
                              {fmt(tok)}{' '}
                              <span className="opacity-60">
                                ({fmt(a.inputTokens)}/{fmt(a.outputTokens)})
                              </span>
                            </td>
                            <td className="px-4 py-2.5 text-[var(--color-text-dim)]">
                              {costText(a.costUSD, a.priced, a.estimated, t)}
                            </td>
                          </tr>
                        )
                      })}
                    </tbody>
                  </table>
                </div>
              )}
            </div>
          </>
        )}
      </div>
    </div>
  )
}
