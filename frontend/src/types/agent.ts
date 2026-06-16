// Agent profile, partial patch, usage counters and per-agent tool selection.

import type { ToolDef } from './mcp'

export interface Agent {
  id: string
  name: string
  soul: string
  identity: string
  provider: string
  model: string
  planningMode: string
  thinkingLevel?: string
  // Visual identity for the roster avatar. Both optional — when empty the UI
  // derives a deterministic circular look from the agent id.
  avatar?: string
  color?: string
  mcpEnabled: boolean
  allowedTools: string
  createdAt: number
  updatedAt: number
}

// Editable agent profile fields (PUT /api/agents/{id}). Partial — omitted keys
// are left unchanged on the backend.
export interface AgentPatch {
  name?: string
  soul?: string
  identity?: string
  provider?: string
  model?: string
  planningMode?: string
  thinkingLevel?: string
  avatar?: string
  color?: string
}

export interface AgentUsage {
  day: string
  calls: number
  inputTokens: number
  outputTokens: number
  dailyCallLimit: number
  dailyTokenLimit: number
}

export interface AgentTools {
  mcpEnabled: boolean
  allowedTools: string[]
  catalog: ToolDef[]
}
