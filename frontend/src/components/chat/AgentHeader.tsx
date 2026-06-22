import type { Agent } from '../../types'
import { AgentAvatar } from '../agents/AgentAvatar'

// AgentHeader is the small avatar + name line atop an assistant bubble, telling
// the user which agent produced (or is producing) the turn. Renders nothing when
// the agent is unknown (single-agent sessions leave it off).
export function AgentHeader({ agent }: { agent?: Agent }) {
  if (!agent) return null
  return (
    <div className="mb-1.5 flex items-center gap-2">
      <AgentAvatar agent={agent} size={20} />
      <span className="text-xs font-medium text-[var(--color-text-dim)]">{agent.name}</span>
    </div>
  )
}
