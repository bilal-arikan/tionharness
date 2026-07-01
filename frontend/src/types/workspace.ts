// Workspace identity, per-workspace settings and workspace-wide tool config.

// A single kanban column definition: key is stored on tasks, label is shown,
// color is an optional hex accent for the column header.
export interface BoardColumnDef {
  key: string
  label: string
  color: string
}

export interface Workspace {
  id: string
  name: string
  createdAt: number
  icon?: string
  color?: string
}

// A workspace template (agents + flow blueprint) offered in the create dialog.
export interface WorkspaceTemplate {
  id: string
  name: string
  description: string
  icon: string
  agentCount: number
  hasFlow: boolean
}

// Per-workspace settings (overrides + rename). Resolved from X-Workspace-Id.
export interface WorkspaceSettings {
  id: string
  name: string
  instructions: string
  icon: string
  color: string
  defaultProvider: string
  defaultModel: string
  pauseAutonomy: boolean
  defaultWorkingDir: string
  // Per-workspace appearance overrides (empty = inherit the app-global appearance).
  theme: string
  accent: string
  themePreset: string
  sessionContextEnabled: boolean
  sessionContextEveryTurn: boolean
  sessionContextRecentCount: number
  boardColumns: BoardColumnDef[]
  createdAt: number
  agentCount: number
  sessionCount: number
  taskCount: number
}

export type WorkspaceSettingsPatch = Partial<
  Pick<
    WorkspaceSettings,
    | 'name'
    | 'instructions'
    | 'icon'
    | 'color'
    | 'defaultProvider'
    | 'defaultModel'
    | 'pauseAutonomy'
    | 'defaultWorkingDir'
    | 'theme'
    | 'accent'
    | 'themePreset'
    | 'sessionContextEnabled'
    | 'sessionContextEveryTurn'
    | 'sessionContextRecentCount'
    | 'boardColumns'
  >
>

// Editable per-workspace config files under <workspace>/config/ (runtime
// prompts, instructions, README) — editable on disk or via the Settings UI.
export interface WorkspaceConfig {
  dir: string
  prompts: Record<string, string>
  defaults: Record<string, string>
  promptKeys: string[]
  instructions: string
  readme: string
}

// Partial update; omitted fields unchanged. A prompt written as "" clears the
// file so the compiled-in default takes over.
export interface WorkspaceConfigPatch {
  prompts?: Record<string, string>
  instructions?: string
  readme?: string
}

// A tool in the workspace-wide tools screen, with its active/inactive state.
// `source`/`server`/`label` describe the tool's origin (built-in vs a specific
// MCP server) for grouping and clean labelling; `inputSchema` is the JSON Schema
// of its arguments, used to render the per-tool detail view.
// One of the four per-tool context visibility tiers (see backend tools.Visibility*).
export type ToolVisibility = 'full' | 'summary' | 'name-only' | 'hidden'

export interface WorkspaceTool {
  name: string
  label: string
  description: string
  source: 'builtin' | 'mcp'
  server: string
  // Functional group key for built-ins (e.g. "files", "memory"); empty for MCP
  // tools, which group by server instead.
  category?: string
  enabled: boolean
  // Effective context visibility tier (code default + workspace override).
  visibility: ToolVisibility
  inputSchema?: unknown
  // Concrete sample calls (each a JSON object matching inputSchema) that show usage
  // conventions the schema alone can't express. Surfaced in the detail view.
  examples?: unknown[]
}

export interface WorkspaceTools {
  tools: WorkspaceTool[]
  disabledTools: string[]
  // Per-tool visibility overrides: tool name → tier. Absent → code default.
  toolVisibility: Record<string, ToolVisibility>
}
