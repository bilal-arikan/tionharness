// Agents: CRUD, usage/budget guardrails and per-agent tool access.
import type {
  Agent,
  AgentPatch,
  AgentUsage,
  AgentTools,
  AgentToolAccess,
  AgentToolTier,
  AgentContextPreview,
} from '@/types'
import { req } from './client'

export const agentApi = {
  // Returns the roster INCLUDING agents marked deleted (flagged), because a past
  // conversation must still resolve its author. Callers that offer a choice must
  // filter on `deleted` — useSessionsController does this once, exposing `agents`
  // (live) alongside `allAgents`.
  listAgents: () => req<Agent[]>('/api/agents'),
  createAgent: (data: {
    name: string
    soul?: string
    identity?: string
    provider?: string
    model?: string
    // Required by the backend: there is no blank reasoning level any more, and it
    // will not pick one for the caller (see providers.ValidateThinkingLevel).
    thinkingLevel: string
    avatar?: string
    color?: string
    // Coordinator defaults for the sessions the new agent opens. coordinatorMode
    // defaults OFF server-side; the recipe slug and the coordinator-only prompt
    // are only meaningful while it is on.
    coordinatorMode?: boolean
    coordinatorWorkflow?: string
    coordinatorPrompt?: string
    // When set, the agent is created as a CHILD inheriting every field from
    // this agent; only the name and a non-empty soul are pinned on top.
    parentId?: string
  }) =>
    req<Agent>('/api/agents', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  // Create a child that inherits every field from `id`. bindRole makes it the
  // workspace's customisation of the parent's system role — the way a locked
  // built-in is customised. Returns the created child.
  deriveAgent: (id: string, opts: { name?: string; bindRole?: boolean } = {}) =>
    req<Agent>(`/api/agents/${id}/derive`, { method: 'POST', body: JSON.stringify(opts) }),
  // Returns the updated agent plus an optional warning when the model is not in
  // the price table (P1.3 — unknown model advisory).
  updateAgent: (id: string, patch: AgentPatch) =>
    req<{ agent: Agent; warning?: string }>(`/api/agents/${id}`, {
      method: 'PUT',
      body: JSON.stringify(patch),
    }),
  // Soft-delete an agent: its schedules and owned tasks go, its SESSIONS stay
  // (history renders it as deleted). Rejects with 409 while a turn is in flight.
  deleteAgent: async (id: string) => {
    try {
      return await req<{ deleted: string }>(`/api/agents/${id}`, { method: 'DELETE' })
    } catch (error) {
      if (error instanceof Error && error.message.startsWith('built-in agent cannot be deleted')) {
        throw new Error(
          'Yerleşik ajan silinemez. Değiştirmek için "Özelleştir" ile kalıtım alan bir kopya oluşturun.',
          { cause: error },
        )
      }
      throw error
    }
  },

  // Drop every override on a derived agent so it inherits its parent again.
  // 409 on a locked built-in, 404 on a root agent.
  restoreDefaultAgent: (id: string) =>
    req<Agent>(`/api/agents/${id}/restore-default`, { method: 'POST' }),

  // Full-copy an agent (profile + provider/model + tool config + skills) into a
  // new "(kopya)" with a fresh id. Returns the created clone.
  duplicateAgent: (id: string) => req<Agent>(`/api/agents/${id}/duplicate`, { method: 'POST' }),

  agentUsage: (agentId: string) => req<AgentUsage>(`/api/agents/${agentId}/usage`),

  agentContext: (agentId: string, message?: string) =>
    req<AgentContextPreview>(
      `/api/agents/${agentId}/context${message ? `?message=${encodeURIComponent(message)}` : ''}`,
    ),

  // Absolute path of the agent's on-disk JSON file.
  agentPath: (agentId: string) => req<{ path: string }>(`/api/agents/${agentId}/path`),

  agentTools: (agentId: string) => req<AgentTools>(`/api/agents/${agentId}/tools`),
  // Read-only "what can this agent use right now": eager vs lazy (gateway-
  // activatable) tools + the MCP server inventory. Purely informational.
  agentToolAccess: (agentId: string) => req<AgentToolAccess>(`/api/agents/${agentId}/tool-access`),
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
