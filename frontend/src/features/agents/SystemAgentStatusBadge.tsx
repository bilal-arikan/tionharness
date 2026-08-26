import type { Agent } from '@/types'

interface Props {
  agent: Agent
}

export function SystemAgentStatusBadge({ agent }: Props) {
  if (!agent.system || !agent.disabled) return null

  return (
    <span
      data-testid="system-agent-fallback-badge"
      className="ml-1.5 shrink-0 rounded bg-[color-mix(in_srgb,var(--color-warning)_18%,transparent)] px-1.5 py-0.5 text-[10px] text-[var(--color-warning)]"
      title="Workspace ajanı devre dışı; oturumlar yerleşik tanımı kullanıyor"
    >
      yerleşik tanım etkin
    </span>
  )
}
