import type { Agent, AgentPatch, AgentTools, AgentToolTier, AgentContextPreview } from '@/types'
import { req } from './client'

export interface AgentAssignment {
  workspaceId: string
  workspaceName: string
  agentId: string
  archived?: boolean
}

export interface CatalogAgent {
  agent: Agent
  assignments: AgentAssignment[]
}

export interface CatalogWorkspace {
  id: string
  name: string
  degraded?: boolean
  degradedReason?: string
}

export const agentCatalogApi = {
  deleteCatalogAgent: (id: string) =>
    req<{ deleted: string }>(`/api/agent-catalog/${id}`, { method: 'DELETE' }),
  createCatalogAgent: (data: {
    name: string
    provider: string
    model: string
    thinkingLevel: string
  }) => req<Agent>('/api/agent-catalog', { method: 'POST', body: JSON.stringify(data) }),
  listAgentCatalog: () =>
    req<{ agents: CatalogAgent[]; workspaces: CatalogWorkspace[] }>('/api/agent-catalog'),
  updateCatalogAgent: (id: string, patch: AgentPatch) =>
    req<{ agent: Agent; warning?: string }>(`/api/agent-catalog/${id}`, {
      method: 'PUT',
      body: JSON.stringify(patch),
    }),
  assignCatalogAgent: (id: string, workspaceId: string) =>
    req<Agent>(`/api/agent-catalog/${id}/workspaces/${encodeURIComponent(workspaceId)}`, {
      method: 'PUT',
    }),
  detachCatalogAgent: (id: string, workspaceId: string) =>
    req<{ detached: boolean }>(
      `/api/agent-catalog/${id}/workspaces/${encodeURIComponent(workspaceId)}`,
      { method: 'DELETE' },
    ),
  restoreCatalogAgent: (id: string) =>
    req<Agent>(`/api/agent-catalog/${id}/restore-default`, { method: 'POST' }),
  deriveCatalogAgent: (id: string, opts: { bindRole: boolean }) =>
    req<Agent>(`/api/agent-catalog/${id}/derive`, { method: 'POST', body: JSON.stringify(opts) }),
  duplicateCatalogAgent: (id: string) =>
    req<Agent>(`/api/agent-catalog/${id}/duplicate`, { method: 'POST' }),
}

// Auxiliary editors use the same central identity as the profile form.
export const catalogEditorApi = {
  agentTools: (id: string) => req<AgentTools>(`/api/agent-catalog/${id}/tools`),
  setAgentTools: (id: string, mcpEnabled: boolean, toolOverrides: Record<string, AgentToolTier>) =>
    req<{ mcpEnabled: boolean; toolOverrides: Record<string, AgentToolTier> }>(
      `/api/agent-catalog/${id}/tools`,
      { method: 'POST', body: JSON.stringify({ mcpEnabled, toolOverrides }) },
    ),
  agentBuiltinPrompt: (id: string) =>
    req<{ systemKey: string; soul: string }>(`/api/agent-catalog/${id}/builtin-prompt`),
  agentContext: (id: string, message?: string) =>
    req<AgentContextPreview>(
      `/api/agent-catalog/${id}/context${message ? `?message=${encodeURIComponent(message)}` : ''}`,
    ),
}
