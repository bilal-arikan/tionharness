import type { Agent } from '@/types'

/** True when an agent may start / own a chat session the user opens.
 *
 * Only the workspace's own agents qualify. System agents never do: service
 * roles (titler, compaction, insight, …) serve the runtime and the backend
 * refuses to make one the default agent (workspace.ErrDefaultAgentSystem), and
 * worker profiles (systemKey "subagent-*", "Worker: Coder" etc.) are what
 * run_subagent delegates to — background executors, not
 * conversation partners. Archived agents are never startable.
 */
export function isStartableAgent(agent: Agent): boolean {
  // An archived agent cannot run (the backend answers 409), so never offer it.
  if (agent.archived) return false
  return !agent.system
}

/** The composer's agent dropdown: the startable roster, plus the session's
 * current agent when it is not in it (a subagent or system session opened from
 * the background tab), so the picker can still show who answers there. */
export function selectableChatAgents(
  startable: Agent[],
  all: Agent[],
  currentAgentId: string | null | undefined,
): Agent[] {
  if (!currentAgentId || startable.some((a) => a.id === currentAgentId)) return startable
  const current = all.find((a) => a.id === currentAgentId)
  return current ? [current, ...startable] : startable
}

/** The roster subset a session-start surface may offer. */
export function startableAgents(agents: Agent[]): Agent[] {
  return agents.filter(isStartableAgent)
}
