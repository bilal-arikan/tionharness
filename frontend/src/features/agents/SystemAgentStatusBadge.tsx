import { Lock } from 'lucide-react'
import type { Agent } from '@/types'

interface Props {
  agent: Agent
}

// SystemAgentStatusBadge explains a system agent's place in the role
// resolution at a glance:
//  - a LOCKED built-in gets a lock glyph (values fixed in the app);
//  - an enabled customisation (System && !Locked) is the row that serves the
//    role right now;
//  - a disabled customisation is dormant — the built-in serves the role.
export function SystemAgentStatusBadge({ agent }: Props) {
  if (!agent.system) return null
  if (agent.locked) {
    return (
      <span
        data-testid="system-agent-locked-badge"
        className="ml-1.5 inline-flex shrink-0 items-center gap-0.5 rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-[10px] text-[var(--color-text-dim)]"
        title="Yerleşik sistem ajanı — değerleri uygulama içinde sabittir; değiştirmek için Özelleştir"
      >
        <Lock size={9} /> yerleşik
      </span>
    )
  }
  if (agent.disabled) {
    return (
      <span
        data-testid="system-agent-fallback-badge"
        className="ml-1.5 shrink-0 rounded bg-[color-mix(in_srgb,var(--color-warning)_18%,transparent)] px-1.5 py-0.5 text-[10px] text-[var(--color-warning)]"
        title="Özelleştirme devre dışı; rolü yerleşik tanım sağlıyor"
      >
        yerleşik tanım etkin
      </span>
    )
  }
  return (
    <span
      data-testid="system-agent-customization-badge"
      className="ml-1.5 shrink-0 rounded bg-[color-mix(in_srgb,var(--color-accent)_18%,transparent)] px-1.5 py-0.5 text-[10px] text-[var(--color-accent)]"
      title={`Bu ajan "${agent.systemKey}" rolünü sağlıyor (yerleşik tanımın yerine)`}
    >
      rolü sağlıyor
    </span>
  )
}
