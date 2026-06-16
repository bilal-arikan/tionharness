// Workspaces (not workspace-scoped) + the native folder picker, plus
// per-workspace settings (active workspace via X-Workspace-Id header).
import type {
  Workspace,
  WorkspaceSettings,
  WorkspaceSettingsPatch,
  WorkspaceConfig,
  WorkspaceConfigPatch,
} from '../types'
import { req } from './client'

export const workspaceApi = {
  listWorkspaces: () => req<Workspace[]>('/api/workspaces'),
  createWorkspace: (data: { name: string; path?: string; icon?: string; color?: string }) =>
    req<Workspace>('/api/workspaces', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  deleteWorkspace: (id: string) =>
    req<{ deleted: string }>(`/api/workspaces/${id}`, { method: 'DELETE' }),
  // Open the OS native folder picker on the backend host (local desktop app).
  pickFolder: () =>
    req<{ path: string; canceled: boolean }>('/api/pick-folder', { method: 'POST' }),

  // Per-workspace settings (active workspace via X-Workspace-Id header).
  getWorkspaceSettings: () => req<WorkspaceSettings>('/api/workspace-settings'),
  updateWorkspaceSettings: (patch: WorkspaceSettingsPatch) =>
    req<WorkspaceSettings>('/api/workspace-settings', {
      method: 'PUT',
      body: JSON.stringify(patch),
    }),

  // Per-workspace editable config files (prompts/instructions/README).
  getWorkspaceConfig: () => req<WorkspaceConfig>('/api/workspace-config'),
  updateWorkspaceConfig: (patch: WorkspaceConfigPatch) =>
    req<WorkspaceConfig>('/api/workspace-config', {
      method: 'PUT',
      body: JSON.stringify(patch),
    }),
  revealWorkspaceConfig: () =>
    req<{ path: string }>('/api/workspace-config/reveal', { method: 'POST' }),
}
