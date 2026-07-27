// Agents: CRUD, usage/budget guardrails and per-agent tool access.
import type {
  Agent,
  AgentPatch,
  AgentUsage,
  AgentTools,
  AgentToolTier,
  AgentContextPreview,
} from '@/types'
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

  agentContext: (agentId: string, message?: string) =>
    req<AgentContextPreview>(
      `/api/agents/${agentId}/context${
        message ? `?message=${encodeURIComponent(message)}` : ''
      }`,
    ),

  // Absolute path of the agent's on-disk JSON file.
  agentPath: (agentId: string) =>
    req<{ path: string }>(`/api/agents/${agentId}/path`),
  // Open the folder holding the agent's JSON file in the OS file manager (local
  // desktop), highlighting the file.
  revealAgent: (agentId: string) =>
    req<{ path: string }>(`/api/agents/${agentId}/reveal`, { method: 'POST' }),

  agentTools: (agentId: string) => req<AgentTools>(`/api/agents/${agentId}/tools`),
  // Replaces the agent's whole override map (tool name → tier, 'blocked'
  // included). An empty map means "no overrides" — every tool follows the
  // workspace-effective tier.
  setAgentTools: (
    agentId: string,
    mcpEnabled: boolean,
    toolOverrides: Record<string, AgentToolTier>,
  ) =>
    req<{ mcpEnabled: boolean; toolOverrides: Record<string, AgentToolTier> }>(
      `/api/agents/${agentId}/tools`,
      { method: 'POST', body: JSON.stringify({ mcpEnabled, toolOverrides }) },
    ),
}
