import type { Agent, AgentPatch } from '@/types'

/** Which roster a bulk edit belongs to. The Agents screen edits the user's own
 * agents and keeps system agents read-only; Settings ▸ System agents edits ONLY
 * system agents (built-ins go through the installation-wide override layer). */
export type AgentBulkScope = 'agents' | 'system'

export function isInBulkScope(agent: Agent, scope: AgentBulkScope): boolean {
  return scope === 'system' ? Boolean(agent.system) : !agent.system
}

export async function updateAgentProviderModels(
  agents: Agent[],
  providerInstanceId: string,
  model: string,
  updateAgent: (id: string, patch: AgentPatch) => Promise<unknown>,
  scope: AgentBulkScope = 'agents',
): Promise<void> {
  if (!providerInstanceId) throw new Error('provider instance is required')
  const editableAgents = agents.filter((agent) => isInBulkScope(agent, scope))
  // Settle every write before reporting a failure, so a caller that re-fetches
  // on error sees the rows that did change rather than racing in-flight writes.
  const results = await Promise.allSettled(
    editableAgents.map((agent) => updateAgent(agent.id, { provider: providerInstanceId, model })),
  )
  const failed = results.find((r): r is PromiseRejectedResult => r.status === 'rejected')
  if (failed) throw failed.reason
}
