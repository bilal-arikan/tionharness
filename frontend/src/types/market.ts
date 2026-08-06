// Marketplace — shareable "packs" (skill / agent / provider / flow / workspace /
// mcp / hook) resolved from four tiers (bundled / global / workspace / remote).
// The catalog carries the manifest only; the kind-specific payload is fetched on
// demand via the detail endpoint and used at install time. See _Docs/21-MARKET.md.

export type PackKind = 'skill' | 'agent' | 'provider' | 'flow' | 'workspace' | 'mcp' | 'hook'
export type PackSource = 'bundled' | 'global' | 'remote'

export interface SkillPayload {
  slug: string
  body: string
}

export interface AgentPayload {
  name: string
  soul?: string
  identity?: string
  provider?: string
  model?: string
  thinkingLevel?: string
  permissionMode?: string
  avatar?: string
  color?: string
  mcpEnabled?: boolean
  allowedTools?: string
  skills?: string[]
}

export interface ProviderPayload {
  label: string
  kind: string
  baseUrl: string
  defaultModel?: string
  models?: string
  // Capability metadata: reasoning-effort support and prompt-cache behaviour
  // ("native" | "auto" | "none" | undefined = unknown). Drives the preview badges.
  reasoning?: boolean
  promptCache?: string
}

export interface FlowPayload {
  name: string
  graph: string
}

export interface BoardColumn {
  key: string
  label: string
  color?: string
}

// A workspace template's starter ecosystem (mirrors the Go market types).
export interface WorkspaceTemplateAgent {
  key: string
  name: string
  soul?: string
  identity?: string
  provider?: string
  model?: string
  thinkingLevel?: string
  permissionMode?: string
  avatar?: string
  color?: string
  mcpEnabled?: boolean
  allowedTools?: string
  /** Legacy per-agent denylist (JSON array); folded into toolOverrides on load. */
  blockedTools?: string
  /** Per-agent tool override map (JSON object: name/pattern → tier | "blocked"). */
  toolOverrides?: string
  skills?: string[]
}

export interface WorkspaceTemplateStep {
  id: string
  title: string
  agentKey: string
  prompt: string
}

export interface WorkspaceTemplateFlow {
  name: string
  steps?: WorkspaceTemplateStep[]
  graph?: string
}

export interface WorkspaceTemplateSchedule {
  name?: string
  agentKey: string
  cronExpr: string
  prompt: string
}

export interface WorkspaceTemplateSkill {
  slug: string
  body: string
  files?: Record<string, string>
}

export interface WorkspacePayload {
  name: string
  icon?: string
  color?: string
  instructions?: string
  columns?: BoardColumn[]
  /** Non-default runtime prompt overrides (registry key → content), seeded under config/prompts/. */
  prompts?: Record<string, string>
  /** Free-form config/README.md shipped with the template. */
  readme?: string
  skills?: WorkspaceTemplateSkill[]
  agents?: WorkspaceTemplateAgent[]
  flows?: WorkspaceTemplateFlow[]
  schedules?: WorkspaceTemplateSchedule[]
}

export interface MCPPayload {
  name: string
  /** One-liner shown in the load-on-demand tool catalog. */
  description?: string
  transport?: string // stdio | http
  command?: string
  args?: string
  url?: string
  envConfig?: string
  /** JSON object of request headers (http transport). */
  headersConfig?: string
  /** Connection scope: "shared" (default) | "scoped" (per session+agent). */
  scope?: string
}

// A lifecycle/tool hook imported from a foreign plugin. Bundled scripts ride in
// the pack's files and ${CLAUDE_PLUGIN_ROOT} is rewritten at install time.
export interface HookPayload {
  event: string
  matcher?: string
  command: string
  timeoutSec?: number
}

export interface PackPayload {
  skill?: SkillPayload
  agent?: AgentPayload
  provider?: ProviderPayload
  flow?: FlowPayload
  workspace?: WorkspacePayload
  mcp?: MCPPayload
  hook?: HookPayload
}

export interface Pack {
  schema: string
  id: string
  kind: PackKind
  name: string
  description: string
  version?: string
  author?: string
  icon?: string
  color?: string
  tags?: string[]
  createdAt?: number
  // Populated only on the detail endpoint (GET /api/market/{id}).
  payload?: PackPayload
  source?: PackSource
  // Set when source === 'remote': the display name of the registry it came from.
  registryName?: string
  // Decorated by the API from the per-workspace install ledger: the version last
  // installed here. Empty = never installed via the market. Compare with version
  // to detect an available update.
  installedVersion?: string
  // Set for directory-site (connector) entries: install runs ingest against this
  // GitHub source instead of downloading a payload. Preview is fetched on demand.
  sourceRef?: { type: string; url: string; keys?: string[] }
}

// Registry is a configured remote pack source (a registry index URL).
export interface Registry {
  name: string
  url: string
  enabled: boolean
  addedAt: number
  /** Built-in directory-site connector id (e.g. "skillsmp"); empty for plain registries. */
  connector?: string
}

export interface InstallResult {
  kind: PackKind
  ref: string
  message: string
}
