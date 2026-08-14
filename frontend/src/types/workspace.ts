// Workspace identity, per-workspace settings and workspace-wide tool config.

import type { TaskPriority } from './task'

// A single kanban column definition: key is stored on tasks, label is shown,
// color is an optional hex accent for the column header.
export interface BoardColumnDef {
  key: string
  label: string
  color: string
}

// Board grouping axis. The board's columns are DERIVED from this: 'status' uses
// the workspace's BoardColumnDef list (classic kanban), the others build columns
// from the tasks themselves. Dragging a card writes the field the axis names.
export type BoardGroupBy = 'status' | 'agent' | 'priority' | 'tag' | 'due'

// Sort order applied within each board column.
export type BoardSort = 'updated' | 'priority' | 'due' | 'deps' | 'title'

// Due-date filter buckets. 'none' matches tasks with no due date at all.
export type BoardDueFilter = 'overdue' | 'today' | 'week' | 'none'

// Dependency filter buckets: 'blocked' = at least one dependency not done,
// 'ready' = has dependencies and all are done.
export type BoardDepFilter = '' | 'blocked' | 'ready'

// Sentinel used in BoardFilter.agentIds to match tasks with no owner agent.
// A real agent id can never be '-', so this cannot collide.
export const UNASSIGNED_AGENT_ID = '-'

// Narrows the board to a subset of tasks. Facets combine with AND; values within
// one facet combine with OR. An empty slice / '' means the facet is inactive, so
// the empty object matches every task.
export interface BoardFilter {
  text?: string
  priorities?: TaskPriority[]
  tags?: string[]
  agentIds?: string[]
  columns?: string[]
  // Multi-select (OR within the facet), so "today OR already late" is expressible.
  dues?: BoardDueFilter[]
  // Single-valued: blocked and ready are mutually exclusive states of one task,
  // so this renders as a radio rather than a checklist.
  dep?: BoardDepFilter
}

// A named filter + layout preset stored per workspace. Built-in views live in
// the client (boardViewTypes.ts) and are never persisted here.
export interface BoardViewDef {
  id: string
  label: string
  icon?: string
  filter: BoardFilter
  groupBy?: BoardGroupBy
  sort?: BoardSort
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
  /** How many seeded agents arrive as coordinators, and how many starter
   *  automation rules ship (always installed disabled). Both 0 on an ordinary
   *  template — the picker only mentions them when non-zero. */
  coordinatorCount?: number
  automationCount?: number
}

// Three-state per-workspace override of the app-global desktop-notification
// master toggle. 'inherit' = follow the global AppSettings value.
export type DesktopNotificationsMode = 'inherit' | 'on' | 'off'

// Per-workspace settings (overrides + rename). Resolved from X-Workspace-Id.
export interface WorkspaceSettings {
  id: string
  name: string
  instructions: string
  icon: string
  color: string
  pauseAutonomy: boolean
  // Default agent pre-selected for new sessions in this workspace.
  defaultAgentId: string
  defaultWorkingDir: string
  // This workspace's resolved claude-cli config home (<workspace>/claude-home),
  // used as CLAUDE_CONFIG_DIR. Read-only/derived (not in the patch); shown in the
  // Providers settings instead of the app-global fallback.
  claudeHomeDir: string
  // Per-workspace appearance overrides (empty = inherit the app-global appearance).
  theme: string
  accent: string
  themePreset: string
  // Appends this workspace's terse ("caveman") reply-style prompt (the editable
  // registry prompt `terse`) to every agent's static system prefix. A reply-style
  // rule only holds when it is always in force, so it is a prompt, not a skill.
  terseMode: boolean
  codebaseMemoryEnabled: boolean
  promptEpochEnabled: boolean
  // In-process shell-output compression (sqz) override: '' = auto (follow sqz-hook
  // detection), 'on' = force on (needs the sqz binary), 'off' = disable.
  shellOutputCompression: '' | 'on' | 'off'
  // In-process shell-COMMAND rewrite (rtk) override, same tri-state. Separate
  // knob: rtk reshapes the command before it runs, sqz compresses the output after.
  shellCommandRewrite: '' | 'on' | 'off'
  boardColumns: BoardColumnDef[]
  // User-created saved board views. Always present (possibly empty); the
  // built-in views are client-side and never round-trip through here.
  boardViews: BoardViewDef[]
  // Keys of post-create advisory cards the user dismissed for this workspace.
  ignoredRecommendations: string[]
  // This workspace's override of the app-global desktop-notification master toggle.
  // Three-state: 'inherit' follows AppSettings.desktopNotifications (default),
  // 'on'/'off' force OS toasts for this workspace regardless of the global toggle.
  desktopNotifications: DesktopNotificationsMode
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
    | 'pauseAutonomy'
    | 'defaultAgentId'
    | 'defaultWorkingDir'
    | 'theme'
    | 'accent'
    | 'themePreset'
    | 'terseMode'
    | 'codebaseMemoryEnabled'
    | 'promptEpochEnabled'
    | 'shellOutputCompression'
    | 'shellCommandRewrite'
    | 'boardColumns'
    | 'boardViews'
    | 'ignoredRecommendations'
    | 'desktopNotifications'
  >
>

// Editable per-workspace config files under <workspace>/config/ (runtime
// prompts, instructions, README) — editable on disk or via the Settings UI.
export interface WorkspaceConfig {
  dir: string
  prompts: Record<string, string>
  defaults: Record<string, string>
  promptKeys: string[]
  promptMeta: Record<string, WorkspacePromptMeta>
  instructions: string
  readme: string
}

// Per-key registry metadata from the central prompt registry (internal/prompts):
// UI label/hint, required {{placeholder}} slots, and whether an edit only lands
// on NEW sessions/epochs (the prompt rides the cached static prefix).
export interface WorkspacePromptMeta {
  label: string
  hint: string
  placeholders?: string[]
  epochAffecting?: boolean
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

// AgentToolTier is the per-agent override scale: the four workspace visibility
// tiers plus 'blocked', which drops the tool from the agent's catalog entirely
// (the successor to the standalone "yasaklı araçlar" denylist).
export type AgentToolTier = ToolVisibility | 'blocked'

export interface WorkspaceTool {
  name: string
  label: string
  description: string
  source: 'builtin' | 'mcp'
  server: string
  // Functional group key for built-ins (e.g. "files", "search"); empty for MCP
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
