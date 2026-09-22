import type { Agent } from '@/types'
import { isWorkerSystemAgent } from '@/features/agents/agentRoster'

/** True when an agent may start / own a chat session the user opens.
 *
 * Service system agents (titler, compaction, insight, …) exist to serve the
 * runtime and are not conversation partners — the backend already refuses to
 * make one the default agent (workspace.ErrDefaultAgentSystem), so offering
 * them at session start only produces a pick the server rejects.
 *
 * Worker profiles (systemKey "subagent-*") are system agents too, but they ARE
 * meant to be addressed directly, so they stay selectable. Archived agents are
 * never startable.
 */
export function isStartableAgent(agent: Agent): boolean {
  // An archived agent cannot run (the backend answers 409), so never offer it.
  if (agent.archived) return false
  if (!agent.system) return true
  return isWorkerSystemAgent(agent)
}

/** The roster subset a session-start surface may offer. */
export function startableAgents(agents: Agent[]): Agent[] {
  return agents.filter(isStartableAgent)
}
