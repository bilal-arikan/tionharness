// MCP servers (workspace-scoped) and workspace-wide tool activation.
import type { MCPServer, MCPTransport, MCPTestResult, WorkspaceTools } from '../types'
import { req } from './client'

export const mcpApi = {
  listMCPServers: () => req<MCPServer[]>('/api/mcp-servers'),
  createMCPServer: (data: {
    name: string
    transport?: MCPTransport
    command?: string
    args?: string[]
    url?: string
    env?: Record<string, string>
  }) =>
    req<MCPServer>('/api/mcp-servers', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  toggleMCPServer: (id: string, enabled: boolean) =>
    req<{ enabled: boolean }>(`/api/mcp-servers/${id}/toggle`, {
      method: 'POST',
      body: JSON.stringify({ enabled }),
    }),
  testMCPServer: (id: string) =>
    req<MCPTestResult>(`/api/mcp-servers/${id}/test`, { method: 'POST' }),
  deleteMCPServer: (id: string) =>
    req<{ result: string }>(`/api/mcp-servers/${id}`, { method: 'DELETE' }),

  // Workspace-wide tool activation (active for the whole workspace; agents pick
  // from the active set).
  workspaceTools: () => req<WorkspaceTools>('/api/workspace-tools'),
  setWorkspaceTools: (disabledTools: string[]) =>
    req<{ disabledTools: string[] }>('/api/workspace-tools', {
      method: 'PUT',
      body: JSON.stringify({ disabledTools }),
    }),
  // Update the visibility override lists (hidden = force load-on-demand, shown =
  // force a default-hidden tool back into context). The denylist is left
  // unchanged (the PUT merges per-field).
  setWorkspaceToolsVisibility: (hiddenTools: string[], shownTools: string[]) =>
    req<{ hiddenTools: string[]; shownTools: string[] }>('/api/workspace-tools', {
      method: 'PUT',
      body: JSON.stringify({ hiddenTools, shownTools }),
    }),
}
