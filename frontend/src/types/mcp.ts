// MCP servers and the tool definitions advertised to the model (Phase 8).

// 'sse' is accepted for legacy rows only; new servers use 'stdio' or 'http'
// (Streamable HTTP). The deprecated sse transport is not dialed by the runtime.
export type MCPTransport = 'stdio' | 'sse' | 'http'

export interface MCPServer {
  id: string
  name: string
  description?: string // short one-liner (rides the per-server catalog summary)
  transport: MCPTransport
  command: string
  args: string // JSON array
  url: string
  envConfig: string // JSON object (env vars, stdio)
  headersConfig: string // JSON object (request headers, http)
  enabled: boolean
  scope: string
  createdAt: number
}

// ImportableMCPServer is one MCP server configured in ANOTHER workspace, offered
// for one-click copy into the current workspace (deduped against what is already
// here). Returned by GET /api/mcp-servers/importable.
export interface ImportableMCPServer {
  workspaceId: string
  workspaceName: string
  server: MCPServer
}

// Result of a bulk mcpServers JSON import: the created rows plus per-entry errors
// keyed by server name (one bad spec does not abort the batch).
export interface MCPImportResult {
  created: MCPServer[]
  errors: Record<string, string>
}

// Live MCP connection-pool snapshot (GET /api/mcp-servers/pool): per-server
// aggregates plus the scoped-connection idle-eviction window (reaper).
interface MCPPoolServerStat {
  server: string
  live: number // alive connections right now
  total: number // pool entries (alive or reconnecting)
  scoped: boolean // has at least one per-(session, agent) connection
}

export interface MCPPoolStats {
  idleSec: number // scoped idle-eviction window in seconds (0 = disabled)
  servers: MCPPoolServerStat[]
}

export interface MCPTestResult {
  ok: boolean
  error?: string
  toolCount?: number
  tools?: { name: string; description: string }[]
}
