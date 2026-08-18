// Workspaces (not workspace-scoped) + the native folder picker, plus
// per-workspace settings (active workspace via X-Workspace-Id header).
import type {
  Workspace,
  WorkspaceTemplate,
  WorkspaceSettings,
  WorkspaceSettingsPatch,
  WorkspaceConfig,
  WorkspaceConfigPatch,
} from '@/types'
import { req } from './client'

export const workspaceApi = {
  listWorkspaces: () => req<Workspace[]>('/api/workspaces'),
  // Cross-workspace live-run flags: for every workspace, whether it currently has
  // any run in flight. Powers the switcher's per-row "çalışıyor" pulse for
  // non-active workspaces (the active one's per-view busy comes from /api/activity).
  listWorkspacesActivity: () => req<{ id: string; running: boolean }[]>('/api/workspaces/activity'),
  // Available workspace templates (agents/flow blueprints) for the create dialog.
  listWorkspaceTemplates: () => req<WorkspaceTemplate[]>('/api/workspace-templates'),
  createWorkspace: (data: {
    name: string
    // Optional project directory (session cwd). The data dir always uses the app default.
    projectDir?: string
    icon?: string
    color?: string
    template?: string
    // Run `git init` (branch "main") in projectDir after creating the workspace.
    // Ignored without a projectDir; an already-versioned folder is left alone.
    gitInit?: boolean
  }) =>
    // The response is the Workspace plus the git-init outcome: the workspace is
    // created either way, so a git failure arrives as gitInitError, not an HTTP error.
    req<Workspace & { gitInit?: boolean; gitInitError?: string }>('/api/workspaces', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  // Adopt an existing workspace data folder (previously created by TionSwarm)
  // by absolute path, registering it without recreating its content. Rejects
  // (throws the backend message) when the folder is not a valid workspace.
  attachWorkspace: (path: string) =>
    req<Workspace>('/api/workspaces/attach', {
      method: 'POST',
      body: JSON.stringify({ path }),
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
  // Pre-flight: probe whether THIS workspace's claude-home is logged in. Spawns a
  // minimal `claude -p` on the backend, so it is on-demand (behind a button).
  checkWorkspaceClaudeAuth: () =>
    req<{ loggedIn: boolean; installed: boolean; claudeHomeDir: string; detail?: string }>(
      '/api/workspace-settings/claude-auth',
    ),
  // Begin an in-app Claude subscription (Max/Pro) OAuth login: returns the
  // authorization URL to open + a flow id to complete with.
  startClaudeOAuth: () =>
    req<{ flowId: string; authUrl: string }>('/api/workspace-settings/claude-auth/oauth/start', {
      method: 'POST',
    }),
  // Finish the OAuth login: exchange the pasted "<code>#<state>" for a credential
  // and write it into THIS workspace's claude-home.
  completeClaudeOAuth: (flowId: string, code: string) =>
    req<{ ok: boolean; claudeHomeDir: string; expiresAt: number; subscription?: string }>(
      '/api/workspace-settings/claude-auth/oauth/complete',
      { method: 'POST', body: JSON.stringify({ flowId, code }) },
    ),
  // Paste-less (loopback) OAuth: the backend binds a local callback listener; the
  // browser redirects straight back to it, so no code paste is needed — poll status.
  startClaudeOAuthLoopback: () =>
    req<{ flowId: string; authUrl: string; port: number }>(
      '/api/workspace-settings/claude-auth/oauth/loopback/start',
      { method: 'POST' },
    ),
  claudeOAuthLoopbackStatus: (flowId: string) =>
    req<{
      status: 'pending' | 'ok' | 'error' | 'unknown'
      detail?: string
      claudeHomeDir?: string
    }>(
      `/api/workspace-settings/claude-auth/oauth/loopback/status?flowId=${encodeURIComponent(flowId)}`,
    ),

  // Pre-flight: probe whether THIS workspace's codex-home is logged in. Cheap
  // filesystem check (auth.json presence), no subprocess spawn — see codex_auth.go.
  checkWorkspaceCodexAuth: () =>
    req<{ loggedIn: boolean; installed: boolean; codexHomeDir: string; detail?: string }>(
      '/api/workspace-settings/codex-auth',
    ),
  // Begin a `codex login --device-auth` flow against THIS workspace's codex-home:
  // returns the verification URL + one-time code to show the user.
  startCodexDeviceAuth: () =>
    req<{ verifyUrl: string; code: string; expiresInSec: number }>(
      '/api/workspace-settings/codex-auth/device/start',
      { method: 'POST' },
    ),
  // Poll the in-flight (or just-finished) device-auth flow's state. 404 = no
  // flow has been started for this codex-home since the process started.
  codexDeviceAuthStatus: () =>
    req<{ state: 'pending' | 'success' | 'failed' | 'expired' | 'cancelled'; error?: string }>(
      '/api/workspace-settings/codex-auth/device/status',
    ),
  // Cancel the in-flight device-auth flow, if any. Idempotent.
  cancelCodexDeviceAuth: () =>
    req<{ ok: boolean }>('/api/workspace-settings/codex-auth/device/cancel', { method: 'POST' }),
  // Log in with a raw API key instead (piped to `codex login --with-api-key`
  // over stdin server-side — never sent as a query param or logged).
  codexAPIKeyLogin: (apiKey: string) =>
    req<{ ok: boolean }>('/api/workspace-settings/codex-auth/api-key', {
      method: 'POST',
      body: JSON.stringify({ apiKey }),
    }),

  // Per-workspace editable config files (prompts/instructions/README).
  getWorkspaceConfig: () => req<WorkspaceConfig>('/api/workspace-config'),
  updateWorkspaceConfig: (patch: WorkspaceConfigPatch) =>
    req<WorkspaceConfig>('/api/workspace-config', {
      method: 'PUT',
      body: JSON.stringify(patch),
    }),
}
