// Application (global) settings, provider/model catalog and read-only prompts.

import type { Locale } from '@/i18n/locales'

// Mirrors settings.DTO — the Anthropic key is never returned; anthropicKeySet
// reports whether one is stored.
export type Theme = 'dark' | 'light' | 'system'

export interface AppSettings {
  theme: Theme
  accent: string
  // Curated palette id (see lib/themePresets). When set, it overrides the full
  // token set; "" falls back to legacy theme + accent behaviour.
  themePreset: string
  // Two independent language axes — see internal/settings/language.go and
  // _Docs/73. `language` is what the AGENT replies in (system-prompt level);
  // `uiLanguage` is the interface chrome, and "" means it follows `language`.
  language: Locale
  uiLanguage: Locale | ''

  defaultPermissionMode: string
  // CLAUDE_CONFIG_DIR for claude-cli subprocesses. Now a FALLBACK only: each turn
  // is overridden to the per-workspace config home (<workspace>/claude-home) so the
  // CLI shares skills/settings/login with its workspace (see _Docs/51). This global
  // value is used solely when no workspace is derivable; shown read-only in the UI.
  claudeConfigDir: string
  // claude-cli credential injected into the subprocess env so an isolated config
  // dir authenticates without an interactive in-dir login. kind selects the env
  // var: "oauth" → CLAUDE_CODE_OAUTH_TOKEN, "apikey" → ANTHROPIC_API_KEY, "" → none.
  claudeCliAuthKind: string
  claudeCliAuthSet: boolean // whether a token is stored (the token itself is never returned)
  // anthropicKeySet reflects the one legacy built-in provider key still live
  // (the ANTHROPIC_API_KEY env boot-seed, see internal/app/app.go). The other
  // legacy typed provider fields (MiniMax/OpenRouter/Z.ai/DeepSeek/custom
  // providers) were removed — those are configured as provider instances now
  // (GET /api/providers), same as any other kind (_Docs/71 Faz 5).
  anthropicKeySet: boolean

  extendedPromptCache: boolean
  anthropicContextEditing: boolean
  anthropicNativeToolSearch: boolean
  anthropicProgrammaticTools: boolean
  anthropicWebTools: boolean
  anthropicServerCompaction: boolean
  anthropicRefusalFallback: boolean
  autonomousTaskBudgetTokens: number

  desktopNotifications: boolean
  keepAwake: boolean

  userName: string
  userTimezone: string
  userCity: string
  userCountry: string
  userNotes: string

  maxContextTokens: number
  keepRecentMsgs: number
  contextBudgetCeil: number
  contextBudgetFraction: number

  // Context reset / handoff (Anthropic "harness design").
  handoffAuto: boolean // auto-reset an autonomous turn that hit the context limit into a fresh session
  handoffMaxChain: number // max consecutive resets before falling back to plain compaction (0 = default 20)
  handoffWriteFile: boolean // also write the handoff to <workdir>/.tionharness/handoff.md

  // Persistent progress (Anthropic claude-progress convention).
  progressPersist: boolean // persist the todo_write checklist to <cwd>/.tionharness/progress.json
  progressResume: boolean // inject a resumed-progress block into a fresh session at start

  // Event-driven session auto-tagging (tool-error/error/goal/goal-done/archived).
  autoTagSessions: boolean

  // Per-session debug journal (parallel observability stream).
  debugJournalEnabled: boolean // emit structured debug events to debug.jsonl
  debugJournalCap: number // newest events kept per session (0 = default 5000)

  // Turn recovery (A1).
  reactiveCompact: boolean
  maxTokenRetries: number
  reactiveKeepRecent: number
  maxOutputTokens: number // generation cap (max_tokens); 0 = auto (per-model family)

  // Self-healing (56-SELF-HEALING).
  maxProviderRetries: number // transient provider-fault retries per turn (0 = disabled, max 5)
  toolGuardWarnings: boolean // append recovery hints to failing tool results
  toolGuardHardStop: boolean // circuit breaker: block repeated identical failures / halt looping turns
  guardExactWarn: number // identical failing call → warn (0 = default 2)
  guardExactBlock: number // identical failing call → block, hard stop only (0 = default 5)
  guardSameToolWarn: number // same-tool consecutive failures → warn (0 = default 3)
  guardSameToolHalt: number // same-tool consecutive failures → halt turn, hard stop only (0 = default 8)
  guardNoProgressWarn: number // identical successful idempotent repeats → warn (0 = default 2)
  guardNoProgressBlock: number // identical successful idempotent repeats → block, hard stop only (0 = default 5)
  stuckTurnThreshold: number // consecutive bad turns before "stuck" tag + autonomous suspension (0 = off)
  lessonReflect: boolean // distill failed turns into stored lessons injected into future turns
  lessonMaxAgeDays: number // prune a lesson not recurring within N days (0 = built-in default)

  autoTitleEnabled: boolean

  // Gated tool capabilities (off by default).
  enableShell: boolean
  enableCliHooks: boolean
  // Code execution with MCP (run_code + generated python bindings); also
  // requires enableShell to take effect. Native path only. _Docs/44.
  enableCodeMode: boolean
  claudeResume: boolean
  claudePersistentSession: boolean
  // How the claude-cli appended system prompt is delivered: false (default) inline
  // via --append-system-prompt, true via a temp file (--append-system-prompt-file).
  claudeSysPromptFile: boolean
  // run_subagent is always installed; availability is per-tool from the Tools screen.
  // These remain as per-turn delegation guards.
  delegationMaxDepth: number
  delegationMaxCalls: number

  // Spawn guards — the detached background surface (run_subagent async + spawn).
  spawnMaxConcurrent: number
  spawnQueueMax: number
  spawnMaxPerTurn: number
  spawnTimeoutMin: number
  spawnIdleTimeoutMin: number
  chatTurnTimeoutMin: number
  chatTurnIdleTimeoutMin: number
  codexStdoutIdleMin: number
  idleResumeMax: number
  scheduleTimeoutMin: number
  turnWatchdogMin: number
  turnIdleWatchdogMin: number

  // Tool execution guards (process-global tool behaviour).
  shellDefaultTimeoutSec: number
  shellMaxTimeoutSec: number
  maxToolOutputKB: number
  // Per-message byte cap (in KB) for send_message / send_to_worker; over it the
  // call fails with an explicit message_too_large error.
  agentMessageMaxKB: number

  // Coordinator/worker guards (M2): active workers per coordinator + auto-turn cap,
  // plus the TREE guards. The per-coordinator worker cap is enforced per node, so
  // nesting multiplies it — coordinatorMaxSubtreeSessions is the one that actually
  // bounds a deep tree. -1 = unlimited on both tree guards.
  coordinatorMaxWorkers: number
  coordinatorMaxTurns: number
  coordinatorMaxDepth: number
  coordinatorMaxSubtreeSessions: number
  // Upward-report backstop delay (seconds). Too short and a slow synthesis turn
  // loses the race, so the parent gets a needless "incomplete"; too long and a
  // stalled branch keeps its coordinator waiting.
  coordinatorSettleGraceSec: number

  // Coordinator stall/hallucination protection: judge-based turn-end guard +
  // staleness sweeper that catch a coordinator narrating a spawn it never issued.
  coordinatorStallGuard: boolean // master on/off (default true)
  coordinatorStallSweepMin: number // staleness sweeper window in minutes (0 = default 5; -1 disables the sweeper)
  coordinatorStallMaxNudges: number // max consecutive corrective nudges (0 = default 2)

  // Working-directory guards for the (unconfined) fs/shell tools.
  autonomousConfine: boolean
  // Inject the boot/verification-sequence reminder on autonomous turns.
  autonomousBootSeq: boolean

  // Workspace backups — periodic zip snapshots of each workspace's data dir.
  backupEnabled: boolean
  backupIntervalHours: number
  backupRetain: number
  backupDir: string // "" → <dataDir>/backups
}

// Partial update. anthropicKey is write-only: "" clears, non-empty sets.
export type SettingsPatch = Partial<
  Omit<AppSettings, 'anthropicKeySet' | 'claudeCliAuthSet'> & {
    anthropicKey: string
    claudeCliAuthToken: string // write-only: "" clears, non-empty stores
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
  // The concrete model id this (possibly alias) id was last OBSERVED to mean —
  // "opus" → "claude-opus-5". Absent until a completed turn revealed it, and
  // absent for providers whose ids are already concrete.
  resolvedModel?: string
  // Reasoning levels this model meaningfully supports ("off"/"low"/"medium"/
  // "high"/"xhigh"/"max"), filled by the backend catalog from ThinkingTiersFor.
  // The pickers keep every tier visible but grey out the ones absent here.
  // Absent means "unknown" → all tiers enabled (the provider clamps anyway).
  thinkingTiers?: string[]
  // How the model handles extended reasoning ("always-on"/"adaptive"/
  // "non-thinking"/"legacy"/"alias"), from the backend ThinkingClass. Paired with
  // thinkingTiers so a greyed tier can explain WHY it is inactive.
  thinkingClass?: string
}

export interface CatalogEntry {
  id: string
  label: string
  needsKey: boolean
  allowCustomModel: boolean
  available: boolean
  models: CatalogModel[]
  // claude-cli only: the locally installed Claude Code version ("2.1.220") and
  // the subscription tier it is logged into ("max" / "pro"). Both are absent for
  // other providers, and either may be absent when it cannot be read.
  cliVersion?: string
  subscription?: string
  // Whether TionHarness's PreToolUse/PostToolUse hooks (and hook-derived
  // behaviour like sqz/PostToolUse token-optimizer compression) fire for this
  // provider's turns. False only for codex-cli, whose subprocess tool loop has
  // no hook passthrough.
  appliesToolHooks: boolean
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

// Result of the advisory release-feed check (GET /api/version/update).
// `state` separates the three outcomes so the UI never has to infer a failed
// check from an empty `latest`:
//   'ok'      → the feed was read; `updateAvailable` says whether to upgrade
//   'skipped' → unstamped dev build, no check was made
//   'unknown' → the feed could not be read (unreachable/non-200/malformed)
// The app never downloads or installs anything; `notesUrl` is the only action.
export interface UpdateStatus {
  state: 'ok' | 'skipped' | 'unknown'
  current: string
  latest: string
  updateAvailable: boolean
  notesUrl: string
  releasedAt: string
  checkedAt: string
  // Direct download for the platform the server runs on, or the generic
  // releases page when the feed lists no build for it. `downloadFile` is the
  // artifact's file name and is empty in that fallback case.
  downloadUrl: string
  downloadFile: string
}

// Detection result for an optional external CLI tool (rtk, sqz, mmdc, piper…).
// The backend resolves the executable's path without running it, then reads the
// version by invoking only the tool's version flag (side-effect free, 3s cap).
// `category` groups tools in the panel; `wire` tells how it is used once present:
//   'hook'    → one-click PreToolUse/PostToolUse toggle
//   'setting' → wired by a workspace setting rather than a hook (rtk)
//   'mcp'     → wired via Settings ▸ MCP (info badge)
//   'cli'     → agent calls it directly via Bash (info badge)
//   'provider'→ runs an LLM provider, configured in Settings ▸ Providers (info badge)
export interface ExternalToolStatus {
  name: string
  desc: string
  url: string
  category: string
  wire: 'hook' | 'setting' | 'mcp' | 'cli' | 'provider'
  found: boolean
  path?: string
  /** Installed version, normalised to `major.minor.patch`. */
  version?: string
  /** Why `version` is empty — shown instead of silently omitting the chip. */
  versionError?: string
  /**
   * How this tool is upgraded. 'command' → TionHarness can run `updateCommand` for
   * the user (a package manager already on the machine). 'manual' → the upgrade
   * replaces a binary or unpacks an archive, which TionHarness refuses to do
   * because a running child locks the file on Windows; `updateNote` says what to
   * do instead.
   */
  updateKind: 'command' | 'manual'
  updateCommand?: string
  updateNote?: string
}

// One tool's upstream release check (POST /api/external-tools/check-updates).
// `status` is 'unknown' whenever either side is unparseable — never a guess.
export interface ExternalToolUpdate {
  name: string
  status: 'up-to-date' | 'outdated' | 'unknown'
  latest?: string
  releaseUrl?: string
  publishedAt?: string
  /** Served from an expired cache because the live fetch failed (offline / rate-limited). */
  stale?: boolean
  error?: string
}

// Result of running one tool's update command.
export interface ExternalToolUpdateResult {
  name: string
  ok: boolean
  output?: string
  version?: string
  versionError?: string
  error?: string
}

// Token-optimizer maintenance payload: each installed tool's OWN `gain` report
// (verbatim — TionHarness does not recompute the figures) plus rtk's config
// location. `rtkConfigExists` is false until `rtk config --create` is run, which
// is normal: rtk runs on built-in defaults until then.
export interface TokenToolReport {
  rtkFound: boolean
  rtkGain?: string
  sqzFound: boolean
  sqzGain?: string
  rtkConfigPath?: string
  rtkConfigExists: boolean
}

// A registered runtime prompt from the central prompt registry
// (internal/prompts), shown read-only in the Komutlar settings screen.
export interface PromptInfo {
  key: string
  label: string
  file: string
  system: string
  user: string
  note: string
  ownedBySystemKey?: string
}

export interface PromptsResponse {
  dir: string
  prompts: PromptInfo[]
}
