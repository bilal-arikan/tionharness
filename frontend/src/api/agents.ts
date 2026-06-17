// Agents: CRUD, usage/budget guardrails and per-agent tool access.
import type { Agent, AgentPatch, AgentUsage, AgentTools, AgentContextPreview } from '../types'
import { req } from './client'

export const agentApi = {
  listAgents: () => req<Agent[]>('/api/agents'),
  createAgent: (data: {
    name: string
    soul?: string
    identity?: string
    provider?: string
    model?: string
    avatar?: string
    color?: string
  }) =>
    req<Agent>('/api/agents', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  updateAgent: (id: string, patch: AgentPatch) =>
    req<Agent>(`/api/agents/${id}`, {
      method: 'PUT',
      body: JSON.stringify(patch),
    }),
  // Delete an agent and the sessions it owns.
  deleteAgent: (id: string) =>
    req<{ deleted: string }>(`/api/agents/${id}`, { method: 'DELETE' }),

  agentUsage: (agentId: string) => req<AgentUsage>(`/api/agents/${agentId}/usage`),
  setBudget: (agentId: string, dailyCallLimit: number, dailyTokenLimit: number) =>
    req<{ dailyCallLimit: number; dailyTokenLimit: number }>(
      `/api/agents/${agentId}/budget`,
      { method: 'POST', body: JSON.stringify({ dailyCallLimit, dailyTokenLimit }) },
    ),

  agentContext: (agentId: string) =>
    req<AgentContextPreview>(`/api/agents/${agentId}/context`),

  agentTools: (agentId: string) => req<AgentTools>(`/api/agents/${agentId}/tools`),
  setAgentTools: (agentId: string, mcpEnabled: boolean, allowedTools: string[]) =>
    req<{ mcpEnabled: boolean; allowedTools: string[] }>(
      `/api/agents/${agentId}/tools`,
      { method: 'POST', body: JSON.stringify({ mcpEnabled, allowedTools }) },
    ),
}
