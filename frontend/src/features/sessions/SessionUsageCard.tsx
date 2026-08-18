import { PiggyBank } from 'lucide-react'
import type { SessionUsageDetail } from '@/types'
import { Section, SaveRow } from './SessionDetailBits'
import { usd, tokens as fmtTok } from '@/shared/lib/format'

// This session's own lifetime spend + savings — the per-conversation
// cost (the session-scoped analog of the agent's daily total below).
export function SessionUsageCard({ sessionUsage }: { sessionUsage: SessionUsageDetail }) {
  return (
    <Section title="Bu oturumun harcaması">
      <div className="mb-2 flex items-baseline gap-2">
        <span className="text-lg font-semibold text-[var(--color-text)]">
          {(sessionUsage.estimated ? '~' : '') + usd(sessionUsage.costUSD)}
        </span>
        <span className="text-[10px] text-[var(--color-text-dim)]">
          {sessionUsage.calls} çağrı ·{' '}
          {fmtTok(sessionUsage.inputTokens + sessionUsage.outputTokens)} token
        </span>
      </div>
      {/* Savings breakdown: prompt-cache USD */}
      <div className="flex flex-col gap-1 rounded-lg border border-[var(--color-border)] bg-[color-mix(in_srgb,var(--color-success)_6%,transparent)] px-2.5 py-2">
        <div className="flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-success)]">
          <PiggyBank size={12} /> Kazanç / tasarruf
        </div>
        {sessionUsage.savingsUSD > 0 && (
          <SaveRow label="Prompt-cache" value={usd(sessionUsage.savingsUSD)} />
        )}
        {sessionUsage.savingsUSD === 0 && (
          <span className="text-[11px] text-[var(--color-text-dim)]">Henüz tasarruf yok.</span>
        )}
      </div>
    </Section>
  )
}
