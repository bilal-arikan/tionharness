import type { WorkerInfo } from '@/types'
import type { AgentLike } from '@/shared/components/agents/AgentIdentity'

// workerAgent maps a worker onto the shape AgentIdentity renders. The name falls
// back to the session title and then the session id so a worker whose agent row
// is gone still reads as something; the visual fields simply stay undefined and
// AgentIdentity draws its own default avatar.
export function workerAgent(w: WorkerInfo): AgentLike {
  return {
    id: w.agentId ?? '',
    name: w.agentName || w.title || w.sessionId,
    avatar: w.agentAvatar,
    color: w.agentColor,
    provider: w.agentProvider,
    model: w.agentModel,
    deleted: w.agentDeleted,
  }
}
