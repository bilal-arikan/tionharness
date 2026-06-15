import { useEffect, useState } from 'react'
import { api } from '../api'
import type { SessionContext, AgentUsage } from '../types'

interface Props {
  agentId: string | null
  sessionId: string | null
  refreshKey: number // bump to re-fetch (e.g. after a chat turn)
  onError: (msg: string) => void
}

function fmtTokens(n: number): string {
  return n >= 1000 ? `${(n / 1000).toFixed(1)}k` : `${n}`
}

// ChatMeters shows a live context-size meter and today's autonomous spend, with
// an inline control to set the agent's daily call limit.
export function ChatMeters({ agentId, sessionId, refreshKey, onError }: Props) {
  const [ctx, setCtx] = useState<SessionContext | null>(null)
  const [usage, setUsage] = useState<AgentUsage | null>(null)

  useEffect(() => {
    if (sessionId) api.sessionContext(sessionId).then(setCtx).catch(() => setCtx(null))
    else setCtx(null)
  }, [sessionId, refreshKey])

  useEffect(() => {
    if (agentId) api.agentUsage(agentId).then(setUsage).catch(() => setUsage(null))
    else setUsage(null)
  }, [agentId, refreshKey])

  const editLimit = async () => {
    if (!agentId || !usage) return
    const input = window.prompt(
      'Günlük otonom çağrı limiti (0 = sınırsız):',
      String(usage.dailyCallLimit),
    )
    if (input === null) return
    const n = parseInt(input, 10)
    if (Number.isNaN(n) || n < 0) return
    try {
      await api.setBudget(agentId, n, usage.dailyTokenLimit)
      setUsage({ ...usage, dailyCallLimit: n })
    } catch (e) {
      onError((e as Error).message)
    }
  }

  if (!agentId) return null

  const overBudget =
    usage && usage.dailyCallLimit > 0 && usage.calls >= usage.dailyCallLimit

  return (
    <div className="flex items-center gap-2 text-xs">
      {ctx && (
        <span
          className="rounded bg-[var(--color-surface-2)] px-2 py-0.5 text-[var(--color-text-dim)]"
          title={
            ctx.hasSummary
              ? `Bağlam ~${ctx.contextTokens} token · ${ctx.summaryMsgCount} mesaj özetlendi`
              : `Bağlam ~${ctx.contextTokens} token`
          }
        >
          ⛁ {fmtTokens(ctx.contextTokens)}
          {ctx.hasSummary && <span className="ml-1 text-[var(--color-accent)]">⧉</span>}
        </span>
      )}
      {usage && (
        <button
          onClick={editLimit}
          className={`rounded px-2 py-0.5 transition hover:opacity-80 ${
            overBudget
              ? 'bg-red-500/15 text-red-400'
              : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)]'
          }`}
          title="Bugünkü otonom çağrılar · tıkla: limit ayarla"
        >
          ◷ {usage.calls}
          {usage.dailyCallLimit > 0 ? `/${usage.dailyCallLimit}` : ''} çağrı
        </button>
      )}
    </div>
  )
}
