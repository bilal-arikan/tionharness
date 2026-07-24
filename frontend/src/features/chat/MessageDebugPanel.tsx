import { useState } from 'react'
import { Activity, X } from 'lucide-react'
import { api } from '@/api'
import type { TurnDebug } from '@/types'

// MessageDebugPanel is the small button at a message's top-left that opens a
// popover with THAT reply's debug/cost/performance detail — token spend, latency,
// cost and the per-tool breakdown — fetched lazily from the per-turn debug rollup
// (correlated by the reply message id). Read-only; shown on assistant turns only.
export function MessageDebugPanel({ sessionId, turnId }: { sessionId: string; turnId: string }) {
  const [open, setOpen] = useState(false)
  const [data, setData] = useState<TurnDebug | null>(null)
  const [err, setErr] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)

  const toggle = () => {
    const next = !open
    setOpen(next)
    if (next && !data && !loading) {
      setLoading(true)
      api
        .sessionTurnDebug(sessionId, turnId)
        .then(setData)
        .catch((e) => setErr((e as Error).message))
        .finally(() => setLoading(false))
    }
  }

  return (
    <div className="relative">
      <button
        onClick={toggle}
        title="Bu mesajın debug · maliyet · performans bilgileri"
        className={`rounded p-0.5 transition hover:text-[var(--color-accent)] ${
          open ? 'text-[var(--color-accent)]' : 'text-[var(--color-text-dim)] opacity-0 group-hover:opacity-100'
        }`}
      >
        <Activity size={13} />
      </button>
      {open && (
        <div className="absolute left-0 top-6 z-30 w-72 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-3 text-xs shadow-[var(--shadow-lg)]">
          <div className="mb-2 flex items-center justify-between">
            <span className="font-semibold">Mesaj debug</span>
            <button
              onClick={() => setOpen(false)}
              className="text-[var(--color-text-dim)] hover:text-[var(--color-text)]"
            >
              <X size={13} />
            </button>
          </div>
          {loading && <p className="text-[var(--color-text-dim)]">Yükleniyor…</p>}
          {err && <p className="text-[var(--color-danger)]">{err}</p>}
          {data && !data.found && (
            <p className="text-[var(--color-text-dim)]">
              Bu mesaj için debug verisi yok (debug günlüğü kapalı ya da bu mesaj önce üretilmiş).
            </p>
          )}
          {data && data.found && (() => {
            const totalPrompt = data.inputTokens + data.cacheReadTokens + data.cacheWriteTokens
            const hitRate = totalPrompt > 0 ? data.cacheReadTokens / totalPrompt : 0
            const warm = data.cacheReadTokens > 0
            const tokPerSec = data.durMs > 0 ? Math.round(data.outputTokens / (data.durMs / 1000)) : 0
            return (
            <div className="space-y-2">
              {/* Cold vs warm: did this turn reuse the prompt cache, or pay a full
                  cold write? The single biggest cost signal per message. */}
              <div className="flex items-center gap-1.5">
                <span
                  className={`rounded px-1.5 py-0.5 text-[10px] font-medium ${
                    warm
                      ? 'bg-[color-mix(in_srgb,var(--color-success)_15%,transparent)] text-[var(--color-success)]'
                      : 'bg-[color-mix(in_srgb,var(--color-warning,#d97706)_15%,transparent)] text-[var(--color-warning,#d97706)]'
                  }`}
                  title={warm ? 'Bu tur sıcak prompt cache\'ini yeniden kullandı (ucuz).' : 'Bu tur soğuk başladı — istem cache\'e tam yazıldı (pahalı).'}
                >
                  {warm ? `🔥 sıcak · %${(hitRate * 100).toFixed(0)} cache` : '❄ soğuk'}
                </span>
              </div>
              {data.model && <Row label="Model" value={data.model} mono />}
              <Row label="Süre" value={fmtMs(data.durMs)} />
              <Row
                label="Maliyet"
                value={`${fmtUSD(data.costUSD)}${data.estimated ? ' ≈' : ''}`}
              />
              <Row label="Toplam istem" value={`${fmtTok(totalPrompt)} token`} />
              {tokPerSec > 0 && <Row label="Çıktı hızı" value={`${tokPerSec} tok/s`} />}
              {data.savingsUSD > 0 && (
                <Row label="Cache tasarrufu" value={`${fmtUSD(data.savingsUSD)}`} />
              )}
              <div className="grid grid-cols-2 gap-1">
                <Stat label="Girdi" value={fmtTok(data.inputTokens)} />
                <Stat label="Çıktı" value={fmtTok(data.outputTokens)} />
                {(data.thinkingTokens ?? 0) > 0 && data.outputTokens > 0 && (
                  <Stat
                    label="Düşünme"
                    value={`%${Math.round(((data.thinkingTokens ?? 0) / data.outputTokens) * 100)} · ${fmtTok(data.thinkingTokens ?? 0)}`}
                  />
                )}
                <Stat label="Cache oku" value={fmtTok(data.cacheReadTokens)} />
                <Stat label="Cache yaz" value={fmtTok(data.cacheWriteTokens)} />
              </div>
              {(data.errors > 0 || data.recoveries > 0 || data.compactions > 0) && (
                <div className="flex flex-wrap gap-1">
                  {data.errors > 0 && <Tag tone="danger">{data.errors} hata</Tag>}
                  {data.recoveries > 0 && <Tag tone="warn">{data.recoveries} kurtarma</Tag>}
                  {data.compactions > 0 && <Tag tone="warn">{data.compactions} sıkıştırma</Tag>}
                </div>
              )}
              {data.tools && data.tools.length > 0 && (
                <div>
                  <div className="mb-1 mt-1 font-medium text-[var(--color-text-dim)]">
                    Araçlar ({data.toolCalls})
                  </div>
                  <ul className="max-h-40 space-y-0.5 overflow-y-auto">
                    {data.tools.map((t, i) => (
                      <li key={i} className="flex items-center justify-between gap-2">
                        <code className={t.err ? 'text-[var(--color-danger)]' : 'text-[var(--color-text)]'}>
                          {t.name}
                        </code>
                        <span className="shrink-0 font-mono text-[10px] text-[var(--color-text-dim)]">
                          {t.durMs > 0 ? `${fmtMs(t.durMs)} · ` : ''}
                          {fmtBytes(t.outBytes)}
                          {t.err ? ' · hata' : ''}
                        </span>
                      </li>
                    ))}
                  </ul>
                </div>
              )}
              {data.lastError && (
                <p className="text-[10px] text-[var(--color-danger)]">{data.lastError}</p>
              )}
            </div>
            )
          })()}
        </div>
      )}
    </div>
  )
}

function Row({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="flex items-center justify-between gap-2">
      <span className="text-[var(--color-text-dim)]">{label}</span>
      <span className={mono ? 'font-mono opacity-90' : 'font-medium'}>{value}</span>
    </div>
  )
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded bg-[var(--color-surface-2)] px-1.5 py-1">
      <div className="text-[9px] uppercase tracking-wide text-[var(--color-text-dim)]">{label}</div>
      <div className="font-mono text-[11px]">{value}</div>
    </div>
  )
}

function Tag({ tone, children }: { tone: 'danger' | 'warn'; children: React.ReactNode }) {
  const cls =
    tone === 'danger'
      ? 'bg-[color-mix(in_srgb,var(--color-danger)_15%,transparent)] text-[var(--color-danger)]'
      : 'bg-[color-mix(in_srgb,var(--color-warning,#d97706)_15%,transparent)] text-[var(--color-warning,#d97706)]'
  return <span className={`rounded px-1.5 py-0.5 text-[10px] font-medium ${cls}`}>{children}</span>
}

function fmtTok(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1000) return `${(n / 1000).toFixed(1)}k`
  return `${n}`
}

function fmtBytes(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}MB`
  if (n >= 1000) return `${(n / 1000).toFixed(1)}KB`
  return `${n}B`
}

function fmtUSD(n: number): string {
  if (n === 0) return '$0'
  return n >= 0.01 ? `$${n.toFixed(2)}` : `$${n.toFixed(4)}`
}

function fmtMs(ms: number): string {
  if (ms >= 1000) return `${(ms / 1000).toFixed(1)}s`
  return `${ms}ms`
}
