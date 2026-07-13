// MCP servers (workspace-scoped) and workspace-wide tool activation.
import type {
  MCPServer,
  MCPTransport,
  MCPTestResult,
  MCPImportResult,
  MCPPoolStats,
  WorkspaceTools,
  ImportableMCPServer,
} from '@/types'
import { req } from './client'

export const mcpApi = {
  listMCPServers: () => req<MCPServer[]>('/api/mcp-servers'),
  createMCPServer: (data: {
    name: string
    description?: string
    transport?: MCPTransport
    command?: string
    args?: string[]
    url?: string
    env?: Record<string, string>
    headers?: Record<string, string>
    // "shared" (default) = one workspace-wide connection; "scoped" = a separate
    // idle-evicted connection per (session, agent). Omit for shared.
    scope?: 'shared' | 'scoped'
  }) =>
    req<MCPServer>('/api/mcp-servers', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  // Edit an existing server (PATCH). Same body shape as create; identity and
  // enabled state are preserved server-side. A changed spec re-dials next turn.
  updateMCPServer: (
    id: string,
    data: {
      name: string
      description?: string
      transport?: MCPTransport
      command?: string
      args?: string[]
      url?: string
      env?: Record<string, string>
      headers?: Record<string, string>
      scope?: 'shared' | 'scoped'
    },
  ) =>
    req<MCPServer>(`/api/mcp-servers/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(data),
    }),
  // Live connection-pool snapshot: per-server live/total connection counts and
  // the scoped idle-eviction window. Drives the Tools screen's reaper indicator.
  mcpPoolStats: () => req<MCPPoolStats>('/api/mcp-servers/pool'),
  // Bulk import from a pasted mcpServers JSON document (Claude Code / .mcp.json
  // shape). Accepts either {"mcpServers": {...}} or a bare {name: spec} map. The
  // raw text is forwarded verbatim so the backend owns parsing/validation.
  importMCPServers: (json: string) =>
    req<MCPImportResult>('/api/mcp-servers/import', { method: 'POST', body: json }),
  toggleMCPServer: (id: string, enabled: boolean) =>
    req<{ enabled: boolean }>(`/api/mcp-servers/${id}/toggle`, {
      method: 'POST',
      body: JSON.stringify({ enabled }),
    }),
  testMCPServer: (id: string) =>
    req<MCPTestResult>(`/api/mcp-servers/${id}/test`, { method: 'POST' }),
  deleteMCPServer: (id: string) =>
    req<{ result: string }>(`/api/mcp-servers/${id}`, { method: 'DELETE' }),

  // MCP servers configured in OTHER workspaces, offered for one-click copy into
  // the current one (deduped against what is already here).
  importableMCPServers: () => req<ImportableMCPServer[]>('/api/mcp-servers/importable'),
  addImportableMCPServer: (workspaceId: string, serverId: string) =>
    req<MCPServer>('/api/mcp-servers/importable/add', {
      method: 'POST',
      body: JSON.stringify({ workspaceId, serverId }),
    }),

  // Workspace-wide tool activation (active for the whole workspace; agents pick
  // from the active set).
  workspaceTools: () => req<WorkspaceTools>('/api/workspace-tools'),
  setWorkspaceTools: (disabledTools: string[]) =>
    req<{ disabledTools: string[] }>('/api/workspace-tools', {
      method: 'PUT',
      body: JSON.stringify({ disabledTools }),
    }),
  // Replace the per-tool visibility override map (tool name → "full" | "summary" |
  // "name-only" | "hidden"). The denylist is left unchanged (the PUT merges
  // per-field). A tool absent from the map uses its code default.
  setWorkspaceToolVisibility: (toolVisibility: Record<string, string>) =>
    req<{ toolVisibility: Record<string, string> }>('/api/workspace-tools', {
      method: 'PUT',
      body: JSON.stringify({ toolVisibility }),
    }),
}
