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
  // Tool-use permission gate: "read-only" | "ask" | "auto". Empty = auto.
  permissionMode?: string
  // Visual identity for the roster avatar. Both optional — when empty the UI
  // derives a deterministic circular look from the agent id.
  avatar?: string
  color?: string
  mcpEnabled: boolean
  allowedTools: string
  // Ordered list of skill slugs enabled for this agent (picked from the shared
  // skill library; agents never own skills).
  skills?: string[]
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
  permissionMode?: string
  avatar?: string
  color?: string
  skills?: string[]
}

import type { ModelStat, KindStat } from './usage'

export interface AgentUsage {
  day: string
  calls: number
  inputTokens: number
  outputTokens: number
  cacheReadTokens?: number
  cacheWriteTokens?: number
  byKind?: Record<string, KindStat>
  byModel?: ModelStat[]
  costUSD?: number
  savingsUSD?: number
  priced?: boolean
  estimated?: boolean
  dailyCallLimit: number
  dailyTokenLimit: number
}

export interface AgentTools {
  mcpEnabled: boolean
  allowedTools: string[]
  catalog: ToolDef[]
}

// Fresh-start context preview: the static system prompt + tool catalog an agent
// begins each turn with (dynamic memory/summary/artifacts are added per-turn).
export interface AgentContextPreview {
  system: string
  systemTokens: number
  tools: { name: string; description: string }[]
  toolTokens: number
  // Lazy (on-demand) tools: schemas NOT shipped at turn start; name+desc only.
  // Their token cost is already included in systemTokens (catalog block).
  lazyTools: { name: string; description: string }[]
  // Simulated per-turn dynamic suffix for the optional sample message (recalled
  // memory + cross-session block). Empty when no message / cross-session off.
  dynamic: string
  dynamicTokens: number
  totalTokens: number
}
