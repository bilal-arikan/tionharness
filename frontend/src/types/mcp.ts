// MCP servers and the tool definitions advertised to the model (Phase 8).

// 'sse' is accepted for legacy rows only; new servers use 'stdio' or 'http'
// (Streamable HTTP). The deprecated sse transport is not dialed by the runtime.
export type MCPTransport = 'stdio' | 'sse' | 'http'

export interface MCPServer {
  id: string
  name: string
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

// Result of a bulk mcpServers JSON import: the created rows plus per-entry errors
// keyed by server name (one bad spec does not abort the batch).
export interface MCPImportResult {
  created: MCPServer[]
  errors: Record<string, string>
}

// A tool advertised to the model (built-in or MCP-sourced).
export interface ToolDef {
  name: string
  description: string
  inputSchema?: unknown
}

export interface MCPTestResult {
  ok: boolean
  error?: string
  toolCount?: number
  tools?: { name: string; description: string }[]
}
