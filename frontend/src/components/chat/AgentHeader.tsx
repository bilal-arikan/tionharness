import type { Agent } from '../../types'
import { AgentIdentity } from '../agents/AgentIdentity'

// AgentHeader is the small avatar + name line atop an assistant bubble, telling
// the user which agent produced (or is producing) the turn. Renders nothing when
// the agent is unknown (single-agent sessions leave it off). When onOpenAgent is
// provided the header becomes a button that jumps to the agent's settings page.
export function AgentHeader({
  agent,
  onOpenAgent,
}: {
  agent?: Agent
  onOpenAgent?: (id: string) => void
}) {
  if (!agent) return null
  if (!onOpenAgent) return <AgentIdentity agent={agent} size="sm" className="mb-1.5" />
  return (
    <button
      type="button"
      onClick={() => onOpenAgent(agent.id)}
      title="Ajan ayarlarını aç"
      className="-mx-1 mb-1.5 flex min-w-0 rounded px-1 py-0.5 text-left transition hover:bg-[var(--color-surface-2)]"
    >
      <AgentIdentity agent={agent} size="sm" />
    </button>
  )
}
