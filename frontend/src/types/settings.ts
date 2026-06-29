// Application (global) settings, provider/model catalog and read-only prompts.

// Mirrors settings.DTO — the Anthropic key is never returned; anthropicKeySet
// reports whether one is stored.
export type Theme = 'dark' | 'light' | 'system'

export interface AppSettings {
  theme: Theme
  accent: string
  // Curated palette id (see lib/themePresets). When set, it overrides the full
  // token set; "" falls back to legacy theme + accent behaviour.
  themePreset: string
  language: 'tr' | 'en'

  defaultProvider: string
  defaultModel: string
  defaultPermissionMode: string
  claudeCliPath: string
  anthropicKeySet: boolean
  minimaxKeySet: boolean
  minimaxBaseUrl: string
  openrouterKeySet: boolean
  openrouterBaseUrl: string

  extendedPromptCache: boolean

  desktopNotifications: boolean
  keepAwake: boolean

  userName: string
  userTimezone: string
  userCity: string
  userCountry: string
  userNotes: string

  maxContextTokens: number
  keepRecentMsgs: number
  recallTopN: number
  recallMinScore: number
  contextBudgetCeil: number
  contextBudgetFraction: number

  journalCap: number
  journalMaxLen: number
  reflectionCap: number       // newest reflections kept per agent; older pruned each dream cycle

  // MemGPT-style self-editing memory (C6).
  memoryPressureWarn: number  // context-fill ratio (0..1) above which the agent is warned; 0 = off
  coreMemoryTools: boolean    // offer core_memory_replace/append editing tools

  // Context reset / handoff (Anthropic "harness design").
  handoffAuto: boolean        // auto-reset an autonomous turn that hit the context limit into a fresh session
  handoffPressure: number     // context-fill ratio above which auto-reset is allowed (0 = default 0.90)
  handoffMaxChain: number     // max consecutive resets before falling back to plain compaction (0 = default 20)
  handoffWriteFile: boolean   // also write the handoff to <workdir>/.swarmgo/handoff.md

  // Persistent progress (Anthropic claude-progress convention).
  progressPersist: boolean    // persist the todo_write checklist to <cwd>/.swarmgo/progress.json
  progressResume: boolean     // inject a resumed-progress block into a fresh session at start

  // Per-session debug journal (parallel observability stream).
  debugJournalEnabled: boolean // emit structured debug events to debug.jsonl
  debugJournalCap: number      // newest events kept per session (0 = default 5000)

  autoReflect: boolean
  autoReflectThreshold: number
  autoUserModel: boolean      // HA-1: refresh the "human" core block from journals during the dream cycle

  // Turn recovery (A1).
  reactiveCompact: boolean
  maxTokenRetries: number
  reactiveKeepRecent: number
  maxOutputTokens: number // generation cap (max_tokens); 0 = auto (per-model family)

  // Tool-output token optimization — two independent, parallel systems.
  compactToolOutput: boolean   // System A: deterministic (free)
  compactMaxLines: number
  compactMaxBytes: number
  compactLlmSummary: boolean   // System B: LLM intent-aware summary (costs a call)
  compactLlmThreshold: number
  compactModel: string         // System B model id; "" → title model, then agent's model

  defaultDailyCallLimit: number
  defaultDailyTokenLimit: number

  pauseAutonomy: boolean

  autoTitleEnabled: boolean
  titleModel: string

  mcpGatewayUrl: string

  // Gated tool capabilities (off by default).
  enableShell: boolean
  enableSelfManage: boolean
  enableCliHooks: boolean
  claudeResume: boolean
  claudePersistentSession: boolean
  enableDelegation: boolean
  delegationMaxDepth: number
  delegationMaxCalls: number

  // Spawn guards — the detached background surface (run_subagent async + spawn).
  spawnMaxConcurrent: number
  spawnMaxPerTurn: number

  // Working-directory guards for the (unconfined) fs/shell tools.
  autonomousConfine: boolean
  gitWorktreeIsolation: boolean
  // Inject the boot/verification-sequence reminder on autonomous turns.
  autonomousBootSeq: boolean

  // Workspace backups — periodic zip snapshots of each workspace's data dir.
  backupEnabled: boolean
  backupIntervalHours: number
  backupRetain: number
  backupDir: string // "" → <dataDir>/backups

  logLevel: string
}

// Partial update. anthropicKey/minimaxKey/openrouterKey are write-only: "" clears, non-empty sets.
export type SettingsPatch = Partial<
  Omit<AppSettings, 'anthropicKeySet' | 'minimaxKeySet' | 'openrouterKeySet'> & {
    anthropicKey: string
    minimaxKey: string
    openrouterKey: string
  }
>

// Live backup subsystem status (GET /api/backups). Mirrors backup.Status.
export interface BackupStatus {
  enabled: boolean
  intervalHours: number
  retain: number
  dir: string
  running: boolean
  lastRun: number // unix seconds; 0 = never
  lastError?: string
}

// One archive written by a backup pass.
export interface BackupArchive {
  workspaceId: string
  workspaceName: string
  path: string
  bytes: number
}

// Result of an on-demand backup pass (POST /api/backups/run). Mirrors backup.Result.
export interface BackupResult {
  started: string
  finished: string
  dir: string
  archives: BackupArchive[]
  failures?: Record<string, string>
}

// One archive file on disk (GET /api/backups/archives). Mirrors backup.ArchiveFile.
export interface BackupArchiveFile {
  name: string
  bytes: number
  modified: number // unix seconds
}

// A workspace's archives, newest first. Mirrors backup.WorkspaceArchives.
export interface WorkspaceArchives {
  workspaceId: string
  workspaceName: string
  archives: BackupArchiveFile[]
}

export interface ProviderTestResult {
  ok: boolean
  model?: string
  sample?: string
  error?: string
}

// Provider/model catalog for the UI's pickers.
export interface CatalogModel {
  id: string
  label: string
  description?: string
  // Approximate context window in tokens (0/undefined = unknown). Filled by the
  // backend catalog from the model-family table.
  contextWindow?: number
}

export interface CatalogEntry {
  id: string
  label: string
  needsKey: boolean
  allowCustomModel: boolean
  available: boolean
  models: CatalogModel[]
}

// Build / version metadata returned by GET /api/version.
// All fields fall back to "dev" when the binary is built without -ldflags.
export interface VersionInfo {
  version: string
  commit: string
  buildDate: string
  goVersion: string
  module: string
}

// Detection result for an optional external token-optimization tool (rtk, sqz).
// Presence-only: the backend looks the executable up on PATH, never runs it.
export interface ExternalToolStatus {
  name: string
  desc: string
  url: string
  found: boolean
  path?: string
}

// A built-in runtime prompt (summary/reflect/title), shown read-only in the
// Komutlar settings screen.
export interface PromptInfo {
  key: string
  label: string
  file: string
  system: string
  user: string
  note: string
}

export interface PromptsResponse {
  dir: string
  prompts: PromptInfo[]
}
