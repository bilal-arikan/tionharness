import { useEffect, useState } from 'react'
import { Database, Layers, Clock } from 'lucide-react'
import { api } from '../../api'
import type { SessionContext, AgentUsage } from '../../types'
import { usd } from '../../lib/format'

interface Props {
  agentId: string | null
  sessionId: string | null
  refreshKey: number // bump to re-fetch (e.g. after a chat turn)
  onError: (msg: string) => void
  // Open the Budget screen (the spend pill deep-links there — Motor B's home).
  onOpenBudget: () => void
}

// Local token formatter: caps at "k" (no "M" threshold) — the chat meter shows
// live context which is expected to stay well under a million tokens.
function fmtTokens(n: number): string {
  return n >= 1000 ? `${(n / 1000).toFixed(1)}k` : `${n}`
}

// ChatMeters shows two indicators sharing nothing but the top bar: a LIVE
// context-size meter (Motor A — this session's window fill) and TODAY'S agent
// spend (Motor B — the whole-day recorded ledger, all sessions). The spend pill
// deep-links to the Budget screen.
export function ChatMeters({ agentId, sessionId, refreshKey, onError, onOpenBudget }: Props) {
  const [ctx, setCtx] = useState<SessionContext | null>(null)
  const [usage, setUsage] = useState<AgentUsage | null>(null)
  void onError

  useEffect(() => {
    if (sessionId) api.sessionContext(sessionId).then(setCtx).catch(() => setCtx(null))
    else setCtx(null)
  }, [sessionId, refreshKey])

  useEffect(() => {
    if (agentId) api.agentUsage(agentId).then(setUsage).catch(() => setUsage(null))
    else setUsage(null)
  }, [agentId, refreshKey])

  if (!agentId) return null

  const cost = usage?.costUSD ?? 0
  const costLabel = cost > 0 ? `${usage?.estimated ? '~' : ''}${usd(cost)}` : ''

  return (
    <div className="flex items-center gap-2 text-xs">
      {ctx && (
        <span
          className="inline-flex items-center gap-1 rounded bg-[var(--color-surface-2)] px-2 py-0.5 text-[var(--color-text-dim)]"
          title={
            ctx.hasSummary
              ? `Bu oturumun bağlam doluluğu ~${ctx.contextTokens} token · ${ctx.summaryMsgCount} mesaj özetlendi`
              : `Bu oturumun bağlam doluluğu ~${ctx.contextTokens} token`
          }
        >
          <Database size={12} /> {fmtTokens(ctx.contextTokens)}
          {ctx.hasSummary && <Layers size={12} className="ml-0.5 text-[var(--color-accent)]" />}
        </span>
      )}
      {usage && (
        <button
          onClick={onOpenBudget}
          className="inline-flex items-center gap-1 rounded bg-[var(--color-surface-2)] px-2 py-0.5 text-[var(--color-text-dim)] transition hover:opacity-80"
          title="Bu ajanın BUGÜNKÜ toplam harcaması (tüm oturumlar) · tıkla: Bütçe ekranı"
        >
          <Clock size={12} /> {usage.calls} çağrı
          {costLabel && <span className="opacity-70">· {costLabel}</span>}
        </button>
      )}
    </div>
  )
}
