// Marketplace — shareable "packs" (skill / agent / provider / flow) resolved
// from three tiers (bundled / global / workspace). The catalog carries the
// manifest only; the kind-specific payload is fetched on demand via the detail
// endpoint and used at install time. See _Docs/21-MARKET.md.

export type PackKind =
  | 'skill'
  | 'agent'
  | 'provider'
  | 'flow'
  | 'workspace'
  | 'memory'
  | 'mcp'
export type PackSource = 'bundled' | 'global' | 'workspace' | 'remote'

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
  capabilities?: string
  planningMode?: string
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
  description?: string
  graph: string
}

export interface BoardColumn {
  key: string
  label: string
  color?: string
}

export interface WorkspacePayload {
  name: string
  icon?: string
  color?: string
  instructions?: string
  columns?: BoardColumn[]
}

export interface MCPPayload {
  name: string
  transport?: string
  command?: string
  args?: string
  url?: string
  envConfig?: string
}

export interface MemoryEntry {
  content: string
  kind?: string
}

export interface MemoryPayload {
  entries: MemoryEntry[]
}

export interface PackPayload {
  skill?: SkillPayload
  agent?: AgentPayload
  provider?: ProviderPayload
  flow?: FlowPayload
  workspace?: WorkspacePayload
  memory?: MemoryPayload
  mcp?: MCPPayload
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
