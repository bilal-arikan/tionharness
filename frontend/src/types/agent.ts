// Agent profile, partial patch, usage counters and per-agent tool selection.

import type { CLIOverhead } from './session'
import type { AgentToolTier, ToolVisibility } from './workspace'

export interface Agent {
  id: string
  name: string
  soul: string
  identity: string
  provider: string
  model: string
  thinkingLevel?: string
  // Tool-use permission gate: "read-only" | "ask" | "auto". Empty = auto.
  permissionMode?: string
  // Visual identity for the roster avatar. Both optional — when empty the UI
  // derives a deterministic circular look from the agent id.
  avatar?: string
  color?: string
  mcpEnabled: boolean
  // Legacy allowlist (JSON array); retained for subagent profiles. User-facing
  // agents leave it empty and use blockedTools instead.
  allowedTools: string
  // Per-agent denylist (JSON array): tools switched off for this agent. Empty =
  // all workspace-active tools available.
  blockedTools: string
  // List of skill slugs enabled for this agent (picked from the shared skill
  // library; agents never own skills).
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
}

// One row of the agent tools screen: a workspace-active tool plus the visibility
// tier it gets WITHOUT any per-agent override — the baseline the UI diffs against.
export interface AgentToolEntry {
  name: string
  description: string
  defaultVisibility: ToolVisibility
  inputSchema?: unknown
}

export interface AgentTools {
  mcpEnabled: boolean
  // Per-agent override map: tool name (or "prefix*" pattern) → tier. A tool
  // absent from the map follows its defaultVisibility. This is the single model
  // for both visibility and banning ('blocked').
  toolOverrides: Record<string, AgentToolTier>
  // Derived 'blocked' slice of toolOverrides. Read-only compatibility field.
  blockedTools: string[]
  catalog: AgentToolEntry[]
}

// Fresh-start context preview: the static system prompt + tool catalog an agent
// begins each turn with (dynamic memory/summary/artifacts are added per-turn).
export interface AgentContextPreview {
  // Agent provider — drives provider-aware UI notes (e.g. claude-cli weaves the
  // dynamic suffix into the last user message rather than a separate system block).
  provider: string
  system: string
  systemTokens: number
  // Selected-skills catalog block, split out of the system prompt (still part of
  // the cached static prefix). Empty when the agent has no skills selected.
  skills: string
  skillsTokens: number
  tools: { name: string; description: string }[]
  toolTokens: number
  // Lazy (on-demand) tools: schemas NOT shipped at turn start; name+desc only.
  // Their token cost is already included in systemTokens (catalog block).
  // visibility is the tool's tier ("summary" | "name-only" | "hidden").
  lazyTools: { name: string; description: string; visibility?: string }[]
  // Simulated per-turn dynamic suffix for the optional sample message (recalled
  // memory + cross-session block). Empty when no message / cross-session off.
  dynamic: string
  dynamicTokens: number
  totalTokens: number
  // CLI-wrapper (claude-cli) harness overhead projection that totalTokens does NOT
  // include. Predicted-only for an agent preview (no session → measuredTokens/calls 0).
  cliOverhead?: CLIOverhead
}
