// Workspace identity, per-workspace settings and workspace-wide tool config.

export interface Workspace {
  id: string
  name: string
  createdAt: number
  icon?: string
  color?: string
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
  createdAt: number
  agentCount: number
  sessionCount: number
  taskCount: number
}

export type WorkspaceSettingsPatch = Partial<
  Pick<WorkspaceSettings, 'name' | 'instructions' | 'icon' | 'color' | 'defaultProvider' | 'defaultModel' | 'pauseAutonomy'>
>

// A tool in the workspace-wide tools screen, with its active/inactive state.
export interface WorkspaceTool {
  name: string
  description: string
  enabled: boolean
}

export interface WorkspaceTools {
  tools: WorkspaceTool[]
  disabledTools: string[]
}
