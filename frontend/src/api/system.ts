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
  ExternalToolUpdate,
  ExternalToolUpdateResult,
  TokenToolReport,
  VersionInfo,
  BackupStatus,
  BackupResult,
  WorkspaceArchives,
} from '@/types'
import { req } from './client'

// subscribeEvents opens the global autonomous-event SSE feed via EventSource
// (which reconnects automatically on drop). `onEvent` receives notification
// frames (`notify`); the optional `onStep` receives live turn-activity frames
// (`step`, type === 'session_step') so autonomous/other-window turns render their
// thinking/tool steps live.
//
// Connection is multiplexed at the module level: the first subscriber creates
// the single EventSource, additional subscribers share it, and the source is
// closed when the last subscriber unsubscribes. This keeps N mounted panels
// (NetworkPanel + TaskBoard + future listeners) from holding N independent
// HTTP/1.1 SSE keep-alives against the backend.
type EventCb = (e: AppEvent) => void
type LogCb = (e: LogEntry) => void
let sharedES: EventSource | null = null
const eventSubs = new Set<EventCb>()
const stepSubs = new Set<EventCb>()
const flowNodeSubs = new Set<EventCb>()
const flowNodeStepSubs = new Set<EventCb>()
const logSubs = new Set<LogCb>()
// Resync subscribers: notified when the feed reopens after a drop (see onopen).
const reconnectSubs = new Set<() => void>()
// Whether the shared feed has ever been open, so the first open is not mistaken
// for a reconnect. Reset when the feed is closed for being idle.
let everConnected = false

function ensureConnection(): void {
  if (sharedES) return
  sharedES = new EventSource('/api/events')
  // The browser only auto-reconnects on transport-level drops (readyState stays
  // CONNECTING). A non-2xx from the dev proxy (Vite 502/504 during a backend
  // blip) or any hard error puts the source in CLOSED permanently and it never
  // retries — the live UI silently goes dark while the server is fine. Detect
  // that terminal state and rebuild the connection ourselves after a short
  // backoff, but only while someone still cares (so an idle close stays closed).
  sharedES.onerror = () => {
    if (!sharedES || sharedES.readyState !== EventSource.CLOSED) return
    sharedES = null
    const hasSubs =
      eventSubs.size > 0 ||
      stepSubs.size > 0 ||
      flowNodeSubs.size > 0 ||
      flowNodeStepSubs.size > 0 ||
      logSubs.size > 0
    if (hasSubs) setTimeout(ensureConnection, 2000)
  }
  sharedES.addEventListener('notify', (ev) => {
    let parsed: AppEvent
    try {
      parsed = JSON.parse((ev as MessageEvent).data) as AppEvent
    } catch {
      return // ignore malformed frames
    }
    // Snapshot the sets so a callback that unsubscribes mid-dispatch doesn't
    // skip its peers or mutate the live iteration.
    eventSubs.forEach((cb) => cb(parsed))
  })
  sharedES.addEventListener('step', (ev) => {
    let parsed: AppEvent
    try {
      parsed = JSON.parse((ev as MessageEvent).data) as AppEvent
    } catch {
      return
    }
    stepSubs.forEach((cb) => cb(parsed))
  })
  sharedES.addEventListener('flownode', (ev) => {
    let parsed: AppEvent
    try {
      parsed = JSON.parse((ev as MessageEvent).data) as AppEvent
    } catch {
      return
    }
    flowNodeSubs.forEach((cb) => cb(parsed))
  })
  sharedES.addEventListener('flownodestep', (ev) => {
    let parsed: AppEvent
    try {
      parsed = JSON.parse((ev as MessageEvent).data) as AppEvent
    } catch {
      return
    }
    flowNodeStepSubs.forEach((cb) => cb(parsed))
  })
  sharedES.addEventListener('log', (ev) => {
    let parsed: AppEvent
    try {
      parsed = JSON.parse((ev as MessageEvent).data) as AppEvent
    } catch {
      return
    }
    if (!parsed.log) return
    const entry = parsed.log
    logSubs.forEach((cb) => cb(entry))
  })
  // A RECONNECT (any open after the first) means the feed was down for a while:
  // every frame published in that window is gone for good — the backend bus is
  // fire-and-forget, there is no replay and no Last-Event-ID cursor here. Any view
  // that renders purely from SSE is therefore stale in a way it cannot detect on
  // its own, so we fan a resync signal out and let each consumer refetch.
  // The FIRST open needs no signal: consumers fetch their initial state on mount.
  sharedES.onopen = () => {
    if (everConnected) reconnectSubs.forEach((cb) => cb())
    everConnected = true
  }
  // EventSource auto-reconnects on transport errors; we don't tear it down
  // here so transient drops don't churn N subscribers.
}

function closeIfIdle(): void {
  if (
    eventSubs.size === 0 &&
    stepSubs.size === 0 &&
    flowNodeSubs.size === 0 &&
    flowNodeStepSubs.size === 0 &&
    logSubs.size === 0 &&
    reconnectSubs.size === 0 &&
    sharedES
  ) {
    sharedES.close()
    sharedES = null
    // The next open is a FIRST open again (fresh subscribers fetch their own
    // initial state), so it must not fire a resync.
    everConnected = false
  }
}

// subscribeReconnect registers `cb` to run whenever the shared feed reopens after
// a drop — the "you missed frames, refetch" signal. Consumers that render live
// state purely from SSE (with no polling fallback) should use it to resync.
function subscribeReconnect(cb: () => void): () => void {
  ensureConnection()
  reconnectSubs.add(cb)
  let unsubscribed = false
  return () => {
    if (unsubscribed) return
    unsubscribed = true
    reconnectSubs.delete(cb)
    closeIfIdle()
  }
}

function subscribeEvents(
  onEvent: EventCb,
  onStep?: EventCb,
  onFlowNode?: EventCb,
  onFlowNodeStep?: EventCb,
): () => void {
  ensureConnection()
  eventSubs.add(onEvent)
  if (onStep) stepSubs.add(onStep)
  if (onFlowNode) flowNodeSubs.add(onFlowNode)
  if (onFlowNodeStep) flowNodeStepSubs.add(onFlowNodeStep)
  let unsubscribed = false
  return () => {
    if (unsubscribed) return
    unsubscribed = true
    eventSubs.delete(onEvent)
    if (onStep) stepSubs.delete(onStep)
    if (onFlowNode) flowNodeSubs.delete(onFlowNode)
    if (onFlowNodeStep) flowNodeStepSubs.delete(onFlowNodeStep)
    closeIfIdle()
  }
}

// subscribeLogs receives every captured application log record live over the
// shared SSE feed (event name `log`), so the Logs screen tails without polling.
function subscribeLogs(onLog: LogCb): () => void {
  ensureConnection()
  logSubs.add(onLog)
  let unsubscribed = false
  return () => {
    if (unsubscribed) return
    unsubscribed = true
    logSubs.delete(onLog)
    closeIfIdle()
  }
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

  // Autonomous event feed (task/schedule) — global SSE stream.
  subscribeEvents,
  // Live application-log tail — same SSE stream, `log` event frames.
  subscribeLogs,
  // "The feed reopened after a drop" signal — for SSE-driven views to resync.
  subscribeReconnect,

  // Application + workspace logs (global ring buffer).
  getLogs: (opts?: {
    limit?: number
    level?: string
    q?: string
    component?: string
    session?: string
    since?: number // unix ms, inclusive
    until?: number // unix ms, inclusive
  }) => {
    const p = new URLSearchParams()
    if (opts?.limit) p.set('limit', String(opts.limit))
    if (opts?.level) p.set('level', opts.level)
    if (opts?.q) p.set('q', opts.q)
    if (opts?.component) p.set('component', opts.component)
    if (opts?.session) p.set('session', opts.session)
    if (opts?.since) p.set('since', String(opts.since))
    if (opts?.until) p.set('until', String(opts.until))
    const qs = p.toString()
    return req<LogEntry[]>(`/api/logs${qs ? `?${qs}` : ''}`)
  },

  // On-disk log file path (for the copy-path action).
  logsPath: () => req<{ path: string }>('/api/logs/path'),

  // Workspace-wide budget/usage: today's totals + per-origin breakdown,
  // per-agent table and a daily trend over the last `days` days.
  workspaceUsage: (days = 7) => req<WorkspaceUsage>(`/api/usage?days=${days}`),

  // Per-view "work in progress" flags for the left-nav busy indicators
  // (chat stream / running task / running flow / schedule-triggered run).
  getActivity: () =>
    req<{
      chat: boolean
      task: boolean
      flow: boolean
      schedule: boolean
      executions: boolean
      insights?: boolean
    }>('/api/activity'),

  // Detect optional external tools on the host and read the version each one
  // reports. Path resolution runs nothing; the version probe runs only the
  // tool's own version flag. Fast + local, so the panel calls it on open.
  externalTools: () => req<ExternalToolStatus[]>('/api/external-tools'),

  // Check each installed tool against its latest published GitHub release.
  // Separate from externalTools() because it leaves the machine: results are
  // cached 6h server-side (GitHub allows 60 unauthenticated calls/hour), and
  // `refresh` drops that cache. Never called automatically on open.
  checkExternalToolUpdates: (refresh = false) =>
    req<ExternalToolUpdate[]>(`/api/external-tools/check-updates${refresh ? '?refresh=1' : ''}`, {
      method: 'POST',
    }),

  // Run one tool's update command. Only accepted for `updateKind === 'command'`
  // tools; the rest answer 409 with their manual instructions, because
  // overwriting a binary a running child holds open would break the tool.
  updateExternalTool: (name: string) =>
    req<ExternalToolUpdateResult>(`/api/external-tools/${encodeURIComponent(name)}/update`, {
      method: 'POST',
    }),

  // Token-optimizer MAINTENANCE (rtk / sqz). These are actions, not settings —
  // the tools' own config is machine-global while TionSwarm settings are
  // per-workspace, so their keys are deliberately NOT mirrored into a workspace
  // setting. See internal/api/external_tools_maint.go.
  //
  // The report is each tool's OWN `gain` output, verbatim: TionSwarm does not
  // recompute the numbers, so they cannot drift from the tools' accounting.
  tokenToolReport: () => req<TokenToolReport>('/api/external-tools/token-report'),
  // Clears sqz's dedup cache — what sqz's own help prescribes when stale
  // `§ref:HASH§` pointers start confusing the agent. Stats/history are kept.
  sqzResetCache: () =>
    req<{ output: string }>('/api/external-tools/sqz-reset-cache', { method: 'POST' }),

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
