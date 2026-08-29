import type { Agent, AgentPatch } from '@/types'

export async function updateAgentProviderModels(
  agents: Agent[],
  providerInstanceId: string,
  model: string,
  updateAgent: (id: string, patch: AgentPatch) => Promise<unknown>,
): Promise<void> {
  if (!providerInstanceId) throw new Error('provider instance is required')
  const editableAgents = agents.filter((agent) => !agent.system)
  await Promise.all(
    editableAgents.map((agent) => updateAgent(agent.id, { provider: providerInstanceId, model })),
  )
}
