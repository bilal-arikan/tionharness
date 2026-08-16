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
  // Soft delete: the agent was removed but its conversations survive, so history
  // still resolves its name/avatar. Filtered out of pickers, kept for rendering.
  deleted?: boolean
  deletedAt?: number
  // Legacy allowlist (JSON array); retained for subagent profiles. User-facing
  // agents leave it empty and use blockedTools instead.
  allowedTools: string
  // Per-agent denylist (JSON array): tools switched off for this agent. Empty =
  // all workspace-active tools available.
  blockedTools: string
  // List of skill slugs enabled for this agent (picked from the shared skill
  // library; agents never own skills).
  skills?: string[]
  // Coordinator DEFAULT for the sessions this agent opens: every new session
  // starts in coordinator mode (spawn_worker & co. available) instead of needing
  // a per-thread toggle. Existing sessions are unaffected by a change here —
  // the session's own flag stays the live value.
  coordinatorMode?: boolean
  // Optional coordinator recipe slug pinned on those sessions (a skill with
  // kind=coordinator-workflow). Only meaningful with coordinatorMode.
  coordinatorWorkflow?: string
  // Free-text orchestration guidance injected into the system context ONLY while
  // the session is in coordinator mode, right after the shared coordinator
  // manual. Empty injects nothing at all, so it is free when not coordinating.
  coordinatorPrompt?: string
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
  coordinatorMode?: boolean
  coordinatorWorkflow?: string
  coordinatorPrompt?: string
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
  // Soft delete: the agent was removed but its conversations survive, so history
  // still resolves its name/avatar. Filtered out of pickers, kept for rendering.
  deleted?: boolean
  deletedAt?: number
  // Per-agent override map: tool name (or "prefix*" pattern) → tier. A tool
  // absent from the map follows its defaultVisibility. This is the single model
  // for both visibility and banning ('blocked').
  toolOverrides: Record<string, AgentToolTier>
  // Derived 'blocked' slice of toolOverrides. Read-only compatibility field.
  blockedTools: string[]
  catalog: AgentToolEntry[]
}

// One tool row of the composer's read-only tool inspector (GET
// /api/agents/{id}/tool-access): origin + effective visibility tier, no schema.
export interface ToolAccessEntry {
  name: string
  label: string
  description: string
  source: 'builtin' | 'mcp'
  server: string
  category?: string
  visibility: ToolVisibility
  // Whether the tool occupies prompt context right now: eager tools always do,
  // lazy tools only while catalogued ('hidden' ones are tool_search-only).
  inContext: boolean
}

// Why a server's tools are (not) in the agent's context.
export type ToolAccessServerStatus =
  'in-context' | 'hidden-only' | 'disabled' | 'agent-mcp-off' | 'no-tools'

// One MCP server as the inspector shows it: config identity, how many of ITS
// tools are eager/lazy for this agent, and its live pool connections. A disabled
// server contributes no tools — it is listed so the user sees what could be on.
export interface ToolAccessServer {
  id: string
  name: string
  transport: string
  scope: string
  enabled: boolean
  status: ToolAccessServerStatus
  // eagerCount: full schemas shipped every turn. lazyCount: listed in the
  // load-on-demand catalog (name/summary). hiddenCount: tool_search-only, NOT in
  // the prompt. contextCount = eager + lazy.
  eagerCount: number
  lazyCount: number
  hiddenCount: number
  contextCount: number
  live: number
  total: number
}

// What the agent can actually use right now: the eager set (schemas shipped
// every turn) vs the lazy set (advertised in the load-on-demand catalog and
// activated through the gateway with tool_search/activate_tools).
export interface AgentToolAccess {
  agentId: string
  agentName: string
  provider: string
  mcpEnabled: boolean
  // Soft delete: the agent was removed but its conversations survive, so history
  // still resolves its name/avatar. Filtered out of pickers, kept for rendering.
  deleted?: boolean
  deletedAt?: number
  eager: ToolAccessEntry[]
  lazy: ToolAccessEntry[]
  blocked: string[]
  servers: ToolAccessServer[]
  // Idle window after which a scoped MCP connection is reaped (0 = disabled).
  poolIdleSec: number
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
