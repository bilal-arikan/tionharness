// Application-global endpoints: settings, provider catalog, read-only prompts,
// logs and the autonomous event feed.
import type {
  AppSettings,
  SettingsPatch,
  ProviderTestResult,
  CatalogEntry,
  PromptsResponse,
  LogEntry,
  AppEvent,
  WorkspaceUsage,
  ExternalToolStatus,
  VersionInfo,
  BackupStatus,
  BackupResult,
  WorkspaceArchives,
} from '../types'
import { req } from './client'

// subscribeEvents opens the global autonomous-event SSE feed via EventSource
// (which reconnects automatically on drop). Returns an unsubscribe function.
function subscribeEvents(onEvent: (e: AppEvent) => void): () => void {
  const es = new EventSource('/api/events')
  es.addEventListener('notify', (ev) => {
    try {
      onEvent(JSON.parse((ev as MessageEvent).data) as AppEvent)
    } catch {
      // ignore malformed frames
    }
  })
  return () => es.close()
}

export const systemApi = {
  // Application settings (global).
  getSettings: () => req<AppSettings>('/api/settings'),
  updateSettings: (patch: SettingsPatch) =>
    req<AppSettings>('/api/settings', {
      method: 'PUT',
      body: JSON.stringify(patch),
    }),
  testProvider: (provider: string, model?: string) =>
    req<ProviderTestResult>('/api/settings/test-provider', {
      method: 'POST',
      body: JSON.stringify({ provider, model: model ?? '' }),
    }),

  // Provider/model catalog (for agent + settings pickers).
  getCatalog: () => req<CatalogEntry[]>('/api/catalog'),

  // Built-in runtime prompts (read-only) + the source folder that holds them.
  getPrompts: () => req<PromptsResponse>('/api/prompts'),
  revealPrompts: () => req<{ path: string }>('/api/prompts/reveal', { method: 'POST' }),

  // Autonomous event feed (task/schedule) — global SSE stream.
  subscribeEvents,

  // Application + workspace logs (global ring buffer).
  getLogs: (opts?: { limit?: number; level?: string; q?: string }) => {
    const p = new URLSearchParams()
    if (opts?.limit) p.set('limit', String(opts.limit))
    if (opts?.level) p.set('level', opts.level)
    if (opts?.q) p.set('q', opts.q)
    const qs = p.toString()
    return req<LogEntry[]>(`/api/logs${qs ? `?${qs}` : ''}`)
  },

  // On-disk log file path (copy) + reveal it in the OS file manager (desktop).
  logsPath: () => req<{ path: string }>('/api/logs/path'),
  revealLogs: () => req<{ path: string }>('/api/logs/reveal', { method: 'POST' }),

  // Workspace-wide budget/usage: today's totals + per-origin breakdown,
  // per-agent table and a daily trend over the last `days` days.
  workspaceUsage: (days = 7) => req<WorkspaceUsage>(`/api/usage?days=${days}`),

  // Per-view "work in progress" flags for the left-nav busy indicators
  // (chat stream / running task / running flow / schedule-triggered run).
  getActivity: () =>
    req<{ chat: boolean; task: boolean; flow: boolean; schedule: boolean; executions: boolean }>(
      '/api/activity',
    ),

  // Detect optional external token-optimization tools (rtk, sqz) on the host
  // PATH. Presence-only — the backend never runs or installs them.
  externalTools: () => req<ExternalToolStatus[]>('/api/external-tools'),

  // Build / version info (injected via ldflags at build time; falls back to
  // "dev" for local development builds without explicit versioning).
  getVersion: () => req<VersionInfo>('/api/version'),

  // Workspace backups: live status (config + last run) and an on-demand run.
  // The periodic schedule itself is driven by the settings document.
  getBackupStatus: () => req<BackupStatus>('/api/backups'),
  runBackup: () => req<BackupResult>('/api/backups/run', { method: 'POST' }),
  // Per-workspace archive list and one-click restore (destructive: overwrites
  // the workspace's current data with the archive, then reopens it live).
  listBackupArchives: () => req<WorkspaceArchives[]>('/api/backups/archives'),
  restoreBackup: (workspaceId: string, archive: string) =>
    req<{ ok: boolean; workspaceId: string; archive: string }>('/api/backups/restore', {
      method: 'POST',
      body: JSON.stringify({ workspaceId, archive }),
    }),
  deleteBackupArchive: (workspaceId: string, archive: string) =>
    req<{ ok: boolean; workspaceId: string; archive: string }>('/api/backups/archives', {
      method: 'DELETE',
      body: JSON.stringify({ workspaceId, archive }),
    }),
}
