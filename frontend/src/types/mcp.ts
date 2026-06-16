// MCP servers and the tool definitions advertised to the model (Phase 8).

export type MCPTransport = 'stdio' | 'sse' | 'http'

export interface MCPServer {
  id: string
  name: string
  transport: MCPTransport
  command: string
  args: string // JSON array
  url: string
  envConfig: string // JSON object
  enabled: boolean
  scope: string
  createdAt: number
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
