import { useEffect, useState } from 'react'
import { Clock } from 'lucide-react'
import { api } from '../../api'
import type { AgentUsage } from '../../types'
import { usd } from '../../lib/format'

interface Props {
  agentId: string | null
  sessionId: string | null
  refreshKey: number // bump to re-fetch (e.g. after a chat turn)
  onError: (msg: string) => void
  // Open the Budget screen (the spend pill deep-links there — Motor B's home).
  onOpenBudget: () => void
}

// ChatMeters shows TODAY'S agent spend in the chat top bar (Motor B — the
// whole-day recorded ledger across all sessions). The spend pill deep-links to
// the Budget screen. (The live per-session context-fill meter used to sit here
// too; it now lives only in the session detail panel to keep the top bar clean.)
export function ChatMeters({ agentId, sessionId, refreshKey, onError, onOpenBudget }: Props) {
  const [usage, setUsage] = useState<AgentUsage | null>(null)
  void onError
  void sessionId

  useEffect(() => {
    if (agentId) api.agentUsage(agentId).then(setUsage).catch(() => setUsage(null))
    else setUsage(null)
  }, [agentId, refreshKey])

  if (!agentId) return null

  const cost = usage?.costUSD ?? 0
  const costLabel = cost > 0 ? `${usage?.estimated ? '~' : ''}${usd(cost)}` : ''

  return (
    <div className="flex items-center gap-2 text-xs">
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
