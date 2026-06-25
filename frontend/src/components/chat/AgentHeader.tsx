import type { Agent } from '../../types'
import { AgentIdentity } from '../agents/AgentIdentity'

// AgentHeader is the small avatar + name line atop an assistant bubble, telling
// the user which agent produced (or is producing) the turn. Renders nothing when
// the agent is unknown (single-agent sessions leave it off).
export function AgentHeader({ agent }: { agent?: Agent }) {
  if (!agent) return null
  return <AgentIdentity agent={agent} size="sm" className="mb-1.5" />
}
