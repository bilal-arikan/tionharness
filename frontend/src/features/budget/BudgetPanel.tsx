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
} from 'lucide-react'
import { api } from '@/api'
import { PaneHeader } from '@/shared/components'
import type { KindStat, ProviderStat, BudgetTrendPoint } from '@/types'
import { AgentAvatar } from '@/shared/components/agents/AgentAvatar'
import { kindColor } from '@/shared/lib/palette'
import { tokens as fmt, usd, bytes, approxTokens } from '@/shared/lib/format'
import { useAsync } from '@/shared/hooks/useAsync'

interface Props {
  onError: (msg: string) => void
}

// TrendMetric selects which series the daily trend chart plots. Each maps a
// BudgetTrendPoint to a scalar, a formatter, a bar colour and a tooltip suffix
// so the same chart can show token volume, spend, caching ROI or compaction.
type TrendMetric = 'token' | 'cost' | 'cacheSave' | 'compact'

interface TrendMetricDef {
  key: TrendMetric
  label: string
  value: (p: BudgetTrendPoint) => number
  fmt: (n: number) => string
  color: string
}

const TREND_METRICS: TrendMetricDef[] = [
  {
    key: 'token',
    label: 'Token',
    value: (p) => p.inputTokens + p.outputTokens,
    fmt,
    color: 'var(--color-accent)',
  },
  {
    key: 'cost',
    label: 'Maliyet',
    value: (p) => p.costUSD,
    fmt: usd,
    color: 'var(--color-warning)',
  },
  {
    key: 'cacheSave',
    label: 'Cache tasarrufu',
    value: (p) => p.savingsUSD,
    fmt: usd,
    color: 'var(--color-success)',
  },
  {
    key: 'compact',
    label: 'Sıkıştırma',
    value: (p) => p.compactSavedBytes + p.compactSavedBytesLLM,
    fmt: bytes,
    color: 'var(--color-success)',
  },
]

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

// Turkish labels per call origin; the hue comes from the shared categorical
// palette (lib/palette) so the same kind keeps its colour across the app.
const KIND_LABELS: Record<string, string> = {
  chat: 'Sohbet',
  task: 'Görev',
  schedule: 'Zamanlama',
  flow: 'Akış',
  delegate: 'Delegasyon',
  title: 'Başlık',
  summary: 'Özet',
  reflect: 'Yansıma',
  compact: 'Sıkıştırma',
  other: 'Diğer',
}

function kindMeta(kind: string) {
  return { label: KIND_LABELS[kind] ?? kind, color: kindColor(kind) }
}

function tokensOf(s: KindStat): number {
  return s.inputTokens + s.outputTokens
}

// costText renders a cost cell honoring the priced/estimated flags.
// Subscription providers (claude-cli) have an estimated equivalent-API cost
// shown with a "~" prefix and a tooltip explaining it is not real billing.
function costText(costUSD: number, priced: boolean, estimated?: boolean): React.ReactNode {
  if (priced) return usd(costUSD)
  if (costUSD > 0) {
    const title = estimated
      ? 'Abonelik (claude-cli) — eşdeğer API maliyeti tahmini; gerçek faturalandırma değil'
      : 'Bir kısmı fiyatsız (abonelik/özel model)'
    return <span title={title}>~{usd(costUSD)}</span>
  }
  return <span className="text-[var(--color-text-dim)]">abonelik / fiyatsız</span>
}

// cacheText renders the "read/write" cache token pair, dimmed when zero.
function cacheText(read: number, write: number): React.ReactNode {
  if (read === 0 && write === 0) return <span className="text-[var(--color-text-dim)]">—</span>
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
        <td className="px-4 py-2.5 text-[var(--color-text-dim)]">{p.calls}</td>
        <td className="px-4 py-2.5 text-[var(--color-text-dim)]">
          {fmt(p.inputTokens + p.outputTokens)}{' '}
          <span className="opacity-60">
            ({fmt(p.inputTokens)}/{fmt(p.outputTokens)})
          </span>
        </td>
        <td className="px-4 py-2.5">{cacheText(p.cacheReadTokens, p.cacheWriteTokens)}</td>
        <td className="px-4 py-2.5 text-[var(--color-text-dim)]">
          {p.savingsUSD > 0 ? <span style={{ color: 'var(--color-success)' }}>{usd(p.savingsUSD)}</span> : '—'}
        </td>
        <td className="px-4 py-2.5 text-[var(--color-text)]">{costText(p.costUSD, p.priced, p.estimated)}</td>
      </tr>
      {open &&
        p.models.map((m) => (
          <tr key={m.model} className="border-t border-[var(--color-border)] bg-[var(--color-surface)]">
            <td className="py-2 pl-11 pr-4 text-xs text-[var(--color-text-dim)]">{m.model || '(varsayılan)'}</td>
            <td className="px-4 py-2 text-xs text-[var(--color-text-dim)]">{m.calls}</td>
            <td className="px-4 py-2 text-xs text-[var(--color-text-dim)]">
              {fmt(m.inputTokens + m.outputTokens)}{' '}
              <span className="opacity-60">
                ({fmt(m.inputTokens)}/{fmt(m.outputTokens)})
              </span>
            </td>
            <td className="px-4 py-2 text-xs">{cacheText(m.cacheReadTokens, m.cacheWriteTokens)}</td>
            <td className="px-4 py-2 text-xs text-[var(--color-text-dim)]">
              {m.savingsUSD > 0 ? <span style={{ color: 'var(--color-success)' }}>{usd(m.savingsUSD)}</span> : '—'}
            </td>
            <td className="px-4 py-2 text-xs text-[var(--color-text)]">{costText(m.costUSD, m.priced, m.estimated)}</td>
          </tr>
        ))}
    </>
  )
}

// SummaryCard is one headline metric at the top of the screen.
function SummaryCard({ icon, label, value, sub }: { icon: React.ReactNode; label: string; value: string; sub?: string }) {
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
function SavingsCell({ title, primary, sub, hint }: { title: string; primary: string; sub: string; hint: string }) {
  return (
    <div className="px-4 py-3" title={hint}>
      <div className="text-[11px] text-[var(--color-text-dim)]">{title}</div>
      <div className="mt-0.5 text-xl font-semibold" style={{ color: 'var(--color-success)' }}>
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
  const { data: usage, loading, error, refresh: load } = useAsync(
    () => api.workspaceUsage(days),
    [days],
  )
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

  const metricDef = TREND_METRICS.find((m) => m.key === trendMetric) ?? TREND_METRICS[0]

  const trendMax = useMemo(() => {
    if (!usage) return 0
    return Math.max(1e-9, ...usage.trend.map((p) => metricDef.value(p)))
  }, [usage, metricDef])

  return (
    <div className="flex h-full flex-col bg-[var(--color-surface)]">
      <PaneHeader
        title="Bütçe"
        subtitle={usage ? `· ${usage.day}` : undefined}
        right={
          <>
            <div className="flex overflow-hidden rounded border border-[var(--color-border)] text-xs">
              {[7, 30, 90].map((d) => (
                <button
                  key={d}
                  onClick={() => setDays(d)}
                  className={`px-2 py-1 transition ${
                    days === d
                      ? 'bg-[var(--color-accent)] text-white'
                      : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)] hover:opacity-80'
                  }`}
                >
                  {d}g
                </button>
              ))}
            </div>
            <button
              onClick={() => load()}
              className="rounded border border-[var(--color-border)] bg-[var(--color-surface-2)] p-1.5 text-[var(--color-text-dim)] transition hover:opacity-80"
              title="Yenile"
            >
              <RefreshCw size={14} className={loading ? 'animate-spin' : ''} />
            </button>
          </>
        }
      />
      <div className="flex-1 overflow-y-auto p-3 md:p-5">
      {!usage ? (
        <div className="text-sm text-[var(--color-text-dim)]">{loading ? 'Yükleniyor…' : 'Veri yok.'}</div>
      ) : (
        <>
          {/* Summary cards — today */}
          <div className="mb-5 grid grid-cols-2 gap-3 lg:grid-cols-4">
            <SummaryCard
              icon={<Coins size={12} />}
              label="Bugünkü toplam token"
              value={fmt(totalTokens)}
              sub={`${fmt(usage.totals.inputTokens)} girdi · ${fmt(usage.totals.outputTokens)} çıktı`}
            />
            <SummaryCard icon={<Hash size={12} />} label="Bugünkü çağrı" value={fmt(usage.totals.calls)} />
            <SummaryCard
              icon={<ArrowDownToLine size={12} />}
              label="Girdi token"
              value={fmt(usage.totals.inputTokens)}
            />
            <SummaryCard
              icon={<DollarSign size={12} />}
              label="Tahmini maliyet (bugün)"
              value={`${usage.totals.priced ? '' : '~'}${usd(usage.totals.costUSD)}`}
              sub={
                usage.totals.priced
                  ? 'liste fiyatı tahmini'
                  : usage.totals.estimated
                    ? 'claude-cli: eşdeğer API maliyeti dahil'
                    : 'bir kısmı abonelik/fiyatsız'
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
              <span className="text-[var(--color-warning,#d97706)]">ⓘ</span>
              <span>
                <strong className="text-[var(--color-text)]">claude-cli ek yükü:</strong> Bu sağlayıcı
                kendi sistem promptu + araç şemaları + MCP köprüsünü modele ekler; gerçek girdi,
                bağlam-önizlemesindeki (context-preview) segment tahmininden çok büyüktür. Buradaki
                rakamlar <strong>gerçek faturalanan</strong> tüketimdir (eşdeğer-API maliyeti) —
                önizleme tahminine değil bunlara güvenin. Tek bir mesajın kırılımı için sohbette o
                mesajın debug butonunu kullanın.
                <br />
                <span className="mt-1 inline-block">
                  <strong className="text-[var(--color-text)]">Araç sayısı farkı:</strong> Bilgi
                  ekranı ajanın erişebildiği tüm kataloğu (ör. 129) sayar; bağlam popup'ının{' '}
                  <em>“Araçlar”</em> satırı yalnız her tur şeması gönderilen <em>eager</em> kümedir.
                  Oturum boyunca <code className="rounded bg-[var(--color-surface-2)] px-1">ToolSearch</code>{' '}
                  ile aktive edilen <em>deferred (lazy)</em> araçlar <code className="rounded bg-[var(--color-surface-2)] px-1">--resume</code>{' '}
                  ile sıcak kalıp gerçek girdiyi büyütür ama popup'ın eager sayısına girmez — bkz.
                  popup'taki <em>“Talep-üzerine”</em> chip'i.
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
              label={`Toplam maliyet (son ${days}g)`}
              value={`${usage.cumulative.costUSD > 0 && !usage.totals.priced ? '~' : ''}${usd(usage.cumulative.costUSD)}`}
              sub={`${fmt(usage.cumulative.calls)} çağrı · ${fmt(usage.cumulative.inputTokens + usage.cumulative.outputTokens)} token`}
            />
            <SummaryCard
              icon={<PiggyBank size={12} />}
              label={`Cache tasarrufu (son ${days}g)`}
              value={usd(usage.cumulative.savingsUSD)}
              sub={`${fmt(usage.cumulative.cacheReadTokens)} oku · ${fmt(usage.cumulative.cacheWriteTokens)} yaz`}
            />
            <SummaryCard
              icon={<Percent size={12} />}
              label="Cache isabet oranı"
              value={`${(usage.cumulative.cacheHitRate * 100).toFixed(0)}%`}
              sub="önbellekten okunan istem payı"
            />
            <SummaryCard
              icon={<DollarSign size={12} />}
              label="Tasarrufsuz maliyet"
              value={`${usage.cumulative.noCacheCostUSD > 0 && !usage.totals.priced ? '~' : ''}${usd(usage.cumulative.noCacheCostUSD)}`}
              sub="caching olmasaydı ödenecek"
            />
          </div>

          {/* Tasarruf Merkezi — every optimization's contribution in one place:
              prompt-cache USD savings + tool-output compaction bytes (System A
              deterministic + System B LLM summary). The compaction meters are
              token-equivalent estimates, not real billing. */}
          <div className="mb-5 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)]">
            <div className="flex items-center gap-2 border-b border-[var(--color-border)] px-4 py-2.5">
              <PiggyBank size={15} style={{ color: 'var(--color-success)' }} />
              <span className="text-sm font-medium text-[var(--color-text)]">Tasarruf Merkezi</span>
              <span className="text-xs text-[var(--color-text-dim)]">· son {days}g · tüm optimizasyon kaynakları</span>
            </div>
            <div className="grid grid-cols-1 divide-y divide-[var(--color-border)] sm:grid-cols-3 sm:divide-x sm:divide-y-0">
              {/* Prompt-cache — the only source with real USD billing impact. */}
              <SavingsCell
                title="Prompt-cache"
                primary={usd(usage.cumulative.savingsUSD)}
                sub={`${fmt(usage.cumulative.cacheReadTokens)} token önbellekten · %${(usage.cumulative.cacheHitRate * 100).toFixed(0)} isabet`}
                hint="Statik prefix'in tekrar okunması yerine cache'ten gelmesinin tam girdi fiyatına kıyasla kazandırdığı gerçek USD."
              />
              {/* System A — deterministic tool-output compaction (free, no LLM). */}
              <SavingsCell
                title="Sıkıştırma · kural (Sistem A)"
                primary={bytes(usage.cumulative.compactSavedBytes)}
                sub={`~${fmt(approxTokens(usage.cumulative.compactSavedBytes))} token context'e girmedi`}
                hint="Araç çıktısından dedupe + boş-satır + ortadan kırpma ile model'e gitmeden çıkarılan bayt. Ücretsiz (yerel)."
              />
              {/* System B — LLM intent-aware summary (costs a cheap call). */}
              <SavingsCell
                title="Sıkıştırma · LLM (Sistem B)"
                primary={bytes(usage.cumulative.compactSavedBytesLLM)}
                sub={`~${fmt(approxTokens(usage.cumulative.compactSavedBytesLLM))} token özetle kırpıldı`}
                hint="Ucuz modelle niyet-farkında özetin araç çıktısından çıkardığı bayt. Özet çağrısının kendi maliyeti 'Sıkıştırma' kökeninde."
              />
            </div>
            <div className="border-t border-[var(--color-border)] px-4 py-2 text-[11px] text-[var(--color-text-dim)]">
              Toplam context tasarrufu:{' '}
              <span className="font-medium text-[var(--color-text)]">
                {bytes(usage.cumulative.compactSavedBytes + usage.cumulative.compactSavedBytesLLM)}
              </span>{' '}
              (~{fmt(approxTokens(usage.cumulative.compactSavedBytes + usage.cumulative.compactSavedBytesLLM))} token) + cache{' '}
              <span className="font-medium" style={{ color: 'var(--color-success)' }}>{usd(usage.cumulative.savingsUSD)}</span>.
              Sıkıştırma bayt ölçerdir (token-eşdeğeri tahmini, gerçek faturalandırma değil).
            </div>
          </div>

          <div className="mb-5 grid grid-cols-1 gap-4 lg:grid-cols-2">
            {/* Origin breakdown */}
            <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] p-4">
              <div className="mb-3 text-sm font-medium text-[var(--color-text)]">Köken kırılımı (bugün)</div>
              {kinds.length === 0 ? (
                <div className="text-xs text-[var(--color-text-dim)]">Bugün henüz kullanım yok.</div>
              ) : (
                <div className="space-y-2">
                  {kinds.map(({ kind, st, tokens }) => {
                    const pct = totalTokens > 0 ? (tokens / totalTokens) * 100 : 0
                    const m = kindMeta(kind)
                    return (
                      <div key={kind}>
                        <div className="mb-0.5 flex items-center justify-between text-xs">
                          <span className="flex items-center gap-1.5 text-[var(--color-text)]">
                            <span className="h-2.5 w-2.5 rounded-sm" style={{ background: m.color }} />
                            {m.label}
                          </span>
                          <span className="text-[var(--color-text-dim)]">
                            {fmt(tokens)} · {pct.toFixed(0)}% · {st.calls} çağrı
                          </span>
                        </div>
                        <div className="h-1.5 w-full overflow-hidden rounded-full bg-[var(--color-surface)]">
                          <div className="h-full rounded-full" style={{ width: `${pct}%`, background: m.color }} />
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
                <div className="text-sm font-medium text-[var(--color-text)]">Trend — son {days} gün</div>
                {/* Metric selector: plot the same window as token volume, spend,
                    caching ROI or tool-output compaction. */}
                <div className="flex overflow-hidden rounded border border-[var(--color-border)] text-[11px]">
                  {TREND_METRICS.map((m) => (
                    <button
                      key={m.key}
                      onClick={() => setTrendMetric(m.key)}
                      className={`px-2 py-1 transition ${
                        trendMetric === m.key
                          ? 'bg-[var(--color-accent)] text-white'
                          : 'bg-[var(--color-surface)] text-[var(--color-text-dim)] hover:opacity-80'
                      }`}
                    >
                      {m.label}
                    </button>
                  ))}
                </div>
              </div>
              {usage.trend.length === 0 ? (
                <div className="text-xs text-[var(--color-text-dim)]">Geçmiş veri yok.</div>
              ) : (
                <div className="flex h-32 items-end gap-1">
                  {usage.trend.map((p) => {
                    const v = metricDef.value(p)
                    const h = (v / trendMax) * 100
                    const tok = p.inputTokens + p.outputTokens
                    const compact = p.compactSavedBytes + p.compactSavedBytesLLM
                    return (
                      <div
                        key={p.day}
                        className="flex-1 rounded-t transition-all hover:opacity-80"
                        style={{ height: `${Math.max(2, h)}%`, background: metricDef.color }}
                        title={`${p.day} · ${metricDef.label}: ${metricDef.fmt(v)}\n${fmt(tok)} token · ${p.calls} çağrı · ${usd(p.costUSD)} maliyet${p.savingsUSD > 0 ? ` · cache ${usd(p.savingsUSD)}` : ''}${compact > 0 ? ` · sıkıştırma ${bytes(compact)}` : ''}`}
                      />
                    )
                  })}
                </div>
              )}
              <div className="mt-2 text-[11px] text-[var(--color-text-dim)]">
                {metricDef.label} ·{' '}
                {metricDef.fmt(usage.trend.reduce((a, p) => a + metricDef.value(p), 0))} toplam
              </div>
            </div>
          </div>

          {/* Per-provider breakdown (expandable to per-model detail) */}
          <div className="mb-5 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)]">
            <div className="flex items-center justify-between border-b border-[var(--color-border)] px-4 py-2.5">
              <span className="text-sm font-medium text-[var(--color-text)]">Provider / model (bugün)</span>
              {usage.totals.savingsUSD > 0 && (
                <span
                  className="rounded px-2 py-0.5 text-xs"
                  style={{
                    background: 'color-mix(in srgb, var(--color-success) 15%, transparent)',
                    color: 'var(--color-success)',
                  }}
                  title="Prompt-cache okumalarının tam girdi fiyatına kıyasla sağladığı tasarruf"
                >
                  cache tasarrufu {usd(usage.totals.savingsUSD)}
                </span>
              )}
            </div>
            {usage.byProvider.length === 0 ? (
              <div className="px-4 py-3 text-xs text-[var(--color-text-dim)]">
                Bugün etiketli provider kullanımı yok. (Deploy öncesi kaydedilen kullanım model/provider
                bilgisi taşımaz; yeni turlar burada görünecek.)
              </div>
            ) : (
              <div className="overflow-x-auto">
              <table className="w-full min-w-[36rem] text-sm">
                <thead>
                  <tr className="text-left text-xs text-[var(--color-text-dim)]">
                    <th className="px-4 py-2 font-medium">Provider / Model</th>
                    <th className="px-4 py-2 font-medium">Çağrı</th>
                    <th className="px-4 py-2 font-medium">Token (G/Ç)</th>
                    <th className="px-4 py-2 font-medium">Cache (oku/yaz)</th>
                    <th className="px-4 py-2 font-medium">Tasarruf</th>
                    <th className="px-4 py-2 font-medium">Maliyet</th>
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
              Ajan başına kullanım (bugün)
            </div>
            {usage.agents.length === 0 ? (
              <div className="px-4 py-3 text-xs text-[var(--color-text-dim)]">Ajan yok.</div>
            ) : (
              <div className="overflow-x-auto">
              <table className="w-full min-w-[28rem] text-sm">
                <thead>
                  <tr className="text-left text-xs text-[var(--color-text-dim)]">
                    <th className="px-4 py-2 font-medium">Ajan</th>
                    <th className="px-4 py-2 font-medium">Çağrı</th>
                    <th className="px-4 py-2 font-medium">Token (G/Ç)</th>
                    <th className="px-4 py-2 font-medium">Maliyet</th>
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
                              agent={{ id: a.agentId, name: a.name, avatar: a.avatar, color: a.color }}
                              size={22}
                            />
                            <span className="text-[var(--color-text)]">{a.name || 'İsimsiz'}</span>
                          </div>
                        </td>
                        <td className="px-4 py-2.5 text-[var(--color-text-dim)]">
                          {a.calls}
                        </td>
                        <td className="px-4 py-2.5 text-[var(--color-text-dim)]">
                          {fmt(tok)}{' '}
                          <span className="opacity-60">
                            ({fmt(a.inputTokens)}/{fmt(a.outputTokens)})
                          </span>
                        </td>
                        <td className="px-4 py-2.5 text-[var(--color-text-dim)]">
                          {costText(a.costUSD, a.priced, a.estimated)}
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
