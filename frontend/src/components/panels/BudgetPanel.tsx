import { useEffect, useMemo, useState } from 'react'
import { Wallet, RefreshCw, Coins, Hash, ArrowDownToLine, DollarSign } from 'lucide-react'
import { api } from '../../api'
import type { WorkspaceUsage, KindStat } from '../../types'
import { AgentAvatar } from '../agents/AgentAvatar'

interface Props {
  onError: (msg: string) => void
}

function fmt(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1000) return `${(n / 1000).toFixed(1)}k`
  return `${n}`
}

// usd renders a USD cost. Sub-cent amounts get more precision so tiny spends
// don't all collapse to $0.00.
function usd(n: number): string {
  if (n === 0) return '$0'
  if (n < 0.01) return `$${n.toFixed(4)}`
  return `$${n.toFixed(2)}`
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

// Turkish labels + a stable color per call origin, so the breakdown reads
// consistently and the same kind keeps its hue across the screen.
const KIND_META: Record<string, { label: string; color: string }> = {
  chat: { label: 'Sohbet', color: '#6366f1' },
  task: { label: 'Görev', color: '#0ea5e9' },
  schedule: { label: 'Zamanlama', color: '#14b8a6' },
  flow: { label: 'Akış', color: '#a855f7' },
  heartbeat: { label: 'Nabız', color: '#f59e0b' },
  delegate: { label: 'Delegasyon', color: '#ec4899' },
  title: { label: 'Başlık', color: '#84cc16' },
  summary: { label: 'Özet', color: '#22c55e' },
  reflect: { label: 'Yansıma', color: '#eab308' },
  compact: { label: 'Sıkıştırma', color: '#ef4444' },
  other: { label: 'Diğer', color: '#94a3b8' },
}

function kindMeta(kind: string) {
  return KIND_META[kind] ?? { label: kind, color: '#94a3b8' }
}

function tokensOf(s: KindStat): number {
  return s.inputTokens + s.outputTokens
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

// BudgetPanel is the workspace-wide budget/usage screen: today's totals, a
// per-origin breakdown (the new ByKind data), a daily trend, and a per-agent
// spend table with limit fill bars.
export function BudgetPanel({ onError }: Props) {
  const [usage, setUsage] = useState<WorkspaceUsage | null>(null)
  const [days, setDays] = useState(7)
  const [loading, setLoading] = useState(false)

  const load = (d = days) => {
    setLoading(true)
    api
      .workspaceUsage(d)
      .then(setUsage)
      .catch((e) => onError((e as Error).message))
      .finally(() => setLoading(false))
  }

  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => load(days), [days])

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

  const trendMax = useMemo(() => {
    if (!usage) return 0
    return Math.max(1, ...usage.trend.map((p) => p.inputTokens + p.outputTokens))
  }, [usage])

  return (
    <div className="flex h-full flex-col overflow-y-auto bg-[var(--color-surface)] p-5">
      {/* Header */}
      <div className="mb-4 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Wallet size={18} className="text-[var(--color-accent)]" />
          <h1 className="text-lg font-semibold text-[var(--color-text)]">Bütçe</h1>
          {usage && <span className="text-xs text-[var(--color-text-dim)]">· {usage.day}</span>}
        </div>
        <div className="flex items-center gap-2">
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
        </div>
      </div>

      {!usage ? (
        <div className="text-sm text-[var(--color-text-dim)]">{loading ? 'Yükleniyor…' : 'Veri yok.'}</div>
      ) : (
        <>
          {/* Summary cards — today */}
          <div className="mb-5 flex gap-3">
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
              sub={usage.totals.priced ? 'liste fiyatı tahmini' : 'bir kısmı abonelik/fiyatsız'}
            />
          </div>

          <div className="mb-5 grid grid-cols-2 gap-4">
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
              <div className="mb-3 text-sm font-medium text-[var(--color-text)]">Trend — son {days} gün (token)</div>
              {usage.trend.length === 0 ? (
                <div className="text-xs text-[var(--color-text-dim)]">Geçmiş veri yok.</div>
              ) : (
                <div className="flex h-32 items-end gap-1">
                  {usage.trend.map((p) => {
                    const t = p.inputTokens + p.outputTokens
                    const h = (t / trendMax) * 100
                    return (
                      <div
                        key={p.day}
                        className="flex-1 rounded-t bg-[var(--color-accent)] transition-all hover:opacity-80"
                        style={{ height: `${Math.max(2, h)}%` }}
                        title={`${p.day}: ${fmt(t)} token · ${p.calls} çağrı`}
                      />
                    )
                  })}
                </div>
              )}
            </div>
          </div>

          {/* Per-provider breakdown */}
          <div className="mb-5 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)]">
            <div className="border-b border-[var(--color-border)] px-4 py-2.5 text-sm font-medium text-[var(--color-text)]">
              Provider'a göre (bugün)
            </div>
            {usage.byProvider.length === 0 ? (
              <div className="px-4 py-3 text-xs text-[var(--color-text-dim)]">
                Bugün etiketli provider kullanımı yok. (Deploy öncesi kaydedilen kullanım model/provider
                bilgisi taşımaz; yeni turlar burada görünecek.)
              </div>
            ) : (
              <table className="w-full text-sm">
                <thead>
                  <tr className="text-left text-xs text-[var(--color-text-dim)]">
                    <th className="px-4 py-2 font-medium">Provider</th>
                    <th className="px-4 py-2 font-medium">Çağrı</th>
                    <th className="px-4 py-2 font-medium">Token (G/Ç)</th>
                    <th className="px-4 py-2 font-medium">Tahmini maliyet</th>
                  </tr>
                </thead>
                <tbody>
                  {usage.byProvider.map((p) => (
                    <tr key={p.provider} className="border-t border-[var(--color-border)]">
                      <td className="px-4 py-2.5 text-[var(--color-text)]">{providerLabel(p.provider)}</td>
                      <td className="px-4 py-2.5 text-[var(--color-text-dim)]">{p.calls}</td>
                      <td className="px-4 py-2.5 text-[var(--color-text-dim)]">
                        {fmt(p.inputTokens + p.outputTokens)}{' '}
                        <span className="opacity-60">
                          ({fmt(p.inputTokens)}/{fmt(p.outputTokens)})
                        </span>
                      </td>
                      <td className="px-4 py-2.5 text-[var(--color-text)]">
                        {p.priced ? (
                          usd(p.costUSD)
                        ) : p.costUSD > 0 ? (
                          <span title="Bir kısmı fiyatsız (abonelik/özel model)">~{usd(p.costUSD)}</span>
                        ) : (
                          <span className="text-[var(--color-text-dim)]">abonelik / fiyatsız</span>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
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
              <table className="w-full text-sm">
                <thead>
                  <tr className="text-left text-xs text-[var(--color-text-dim)]">
                    <th className="px-4 py-2 font-medium">Ajan</th>
                    <th className="px-4 py-2 font-medium">Çağrı</th>
                    <th className="px-4 py-2 font-medium">Token (G/Ç)</th>
                    <th className="px-4 py-2 font-medium">Maliyet</th>
                    <th className="px-4 py-2 font-medium">Token limiti doluluk</th>
                    <th className="px-4 py-2 font-medium">Durum</th>
                  </tr>
                </thead>
                <tbody>
                  {usage.agents.map((a) => {
                    const tok = a.inputTokens + a.outputTokens
                    const callOver = a.dailyCallLimit > 0 && a.calls >= a.dailyCallLimit
                    const tokFill = a.dailyTokenLimit > 0 ? Math.min(100, (tok / a.dailyTokenLimit) * 100) : 0
                    const tokOver = a.dailyTokenLimit > 0 && tok >= a.dailyTokenLimit
                    const over = callOver || tokOver
                    const near = !over && a.dailyTokenLimit > 0 && tokFill >= 80
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
                          {a.dailyCallLimit > 0 && <span className="opacity-60">/{a.dailyCallLimit}</span>}
                        </td>
                        <td className="px-4 py-2.5 text-[var(--color-text-dim)]">
                          {fmt(tok)}{' '}
                          <span className="opacity-60">
                            ({fmt(a.inputTokens)}/{fmt(a.outputTokens)})
                          </span>
                        </td>
                        <td className="px-4 py-2.5 text-[var(--color-text-dim)]">
                          {a.priced ? usd(a.costUSD) : a.costUSD > 0 ? `~${usd(a.costUSD)}` : '—'}
                        </td>
                        <td className="px-4 py-2.5">
                          {a.dailyTokenLimit > 0 ? (
                            <div className="flex items-center gap-2">
                              <div className="h-1.5 w-24 overflow-hidden rounded-full bg-[var(--color-surface)]">
                                <div
                                  className="h-full rounded-full"
                                  style={{
                                    width: `${tokFill}%`,
                                    background: over
                                      ? 'var(--color-danger)'
                                      : near
                                        ? 'var(--color-warning)'
                                        : 'var(--color-accent)',
                                  }}
                                />
                              </div>
                              <span className="text-xs text-[var(--color-text-dim)]">{tokFill.toFixed(0)}%</span>
                            </div>
                          ) : (
                            <span className="text-xs text-[var(--color-text-dim)]">sınırsız</span>
                          )}
                        </td>
                        <td className="px-4 py-2.5">
                          <span
                            className="rounded px-2 py-0.5 text-xs"
                            style={{
                              background: over
                                ? 'color-mix(in srgb, var(--color-danger) 15%, transparent)'
                                : near
                                  ? 'color-mix(in srgb, var(--color-warning) 15%, transparent)'
                                  : 'var(--color-surface)',
                              color: over
                                ? 'var(--color-danger)'
                                : near
                                  ? 'var(--color-warning)'
                                  : 'var(--color-text-dim)',
                            }}
                          >
                            {over ? 'Aşıldı' : near ? 'Limit yakın' : 'Normal'}
                          </span>
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            )}
          </div>
        </>
      )}
    </div>
  )
}
