---
name: "TionHarness Settings"
description: "Every application-wide setting in TionHarness (settings.json) — what each field does, its valid range/default — and how to read and change them live with the get_settings / update_settings tools."
when_to_use: "When you need to inspect or change TionHarness's application settings: theme, providers, default model, context budgets, autonomy, compaction, or the gated tool capabilities"
icon: "⚙️"
color: "#8b5cf6"
access: shared
auto_summary: false
---
# TionHarness — Application Settings

TionHarness keeps all application-wide configuration in a single JSON document,
`settings.json`, in the data directory. It backs the Settings screen and is the
single source of truth. Changes are **persisted to that file AND pushed into
every live subsystem immediately** — no restart.

## Tools

Two self-management tools (require the **self-management** capability to be on):

- **`get_settings`** — returns the `settings.json` file path plus the current
  values as JSON. Secret API keys are masked (you only see whether a key is set).
  The field names it prints are exactly the keys `update_settings` expects.
- **`update_settings`** — takes a `patch` object containing only the fields to
  change (e.g. `{"theme":"light"}`), writes them and **activates** them live.
  Numeric fields are clamped to safe ranges.

**Always call `get_settings` first**, then patch the exact keys you saw. A patch
only touches the fields you include; everything else is left unchanged.

```
get_settings → {}
update_settings → {"patch": {"autoTitleEnabled": false}}
```

## Settings reference

### Appearance
- `themePreset` — the theme color + variant id, e.g. `"violet-dark"` / `"violet-light"`
  (default `"violet-dark"`; `""` inherits/falls back to the default). This is the **only**
  appearance control now: the id encodes both the color and the light/dark mode.
- `theme`, `accent` — **deprecated/unused** (the base-mode selector and accent color
  picker were removed 2026-07-01; the variant id drives light/dark). Fields still exist in
  settings.json for backward compat but no longer affect the UI.
- `language` — `"tr"` | `"en"` (default `tr`). Injected into chat turns as a
  reply-language directive inside the "## About the user" block (agents default to
  this language). Not yet injected into autonomous/flow turns.
- `desktopNotifications`, `keepAwake` — booleans (applied client-side).
  `desktopNotifications` here is the app-global **default** master toggle for OS
  toasts. Each workspace may **override** it via the workspace-settings field
  `desktopNotifications` (three-state: `"inherit"` = follow this global default,
  `"on"`/`"off"` = force for that workspace), edited under Settings ▸ Bildirimler.
  The effective gate = workspace override unless `"inherit"`, then the global value.
  Per-*type* mute (task/flow/schedule/agent) stays device-local (localStorage).

### Providers & model
Providers are **not** part of `settings.json` anymore — they moved to a
separate instance model (kind → instance) with their own store and API.
This section only covers what is still a plain `get_settings`/`update_settings`
field; for provider CRUD see below.

- `defaultModel` — model id; `""` = the provider's own default.
- CLI path, config home, and authentication are provider-instance fields, not
  application settings. A newly created `claude-cli` or `codex-cli` instance with
  an empty `configDir` receives `<dataDir>/provider-homes/<instance-id>`
  automatically. It is exported as `CLAUDE_CONFIG_DIR` or `CODEX_HOME`.
- Authenticate the selected instance through `GET /api/providers/{id}/auth` and
  the matching `/api/providers/{id}/auth/...` OAuth/device/API-key routes. The
  Providers panel exposes these as **Giriş yap / Durum**. Legacy
  `/api/workspace-settings/claude-auth...` and `codex-auth...` routes remain only
  for backward compatibility.
- `extendedPromptCache` — Anthropic extended (1h) prompt-cache beta (anthropic only). (The 1M-context beta was retired — 1M is GA since 2026-03, so there is no `oneMillionContext` setting anymore.)
- `anthropicContextEditing` — Anthropic API-native context editing beta (anthropic only, default **off**). The server clears old tool_use/tool_result blocks from the cached prefix in place (`clear_tool_uses_20250919`, the microcompact analogue) once the prompt grows past ~100k input tokens, keeping the newest 3 tool uses. Complements TionHarness's client-side compaction; does not replace it.

#### Provider instances (kind → instance model)

Provider credentials/config live in a separate, app-wide, AES-GCM encrypted
store (`providers.json`), not in `settings.json`. A **kind** (`claude-cli`,
`anthropic`, `minimax`, `minimax-anthropic`, `openrouter`, `zai`, `deepseek`,
`deepseek-anthropic`, `codex-cli`, `openai-compat`, `anthropic-compat`) is a
built-in template that declares its own config form; an **instance** is a
concrete, user-created provider (a kind + label + filled fields + encrypted
secrets). The same kind can have multiple instances (e.g. two `anthropic`
instances with different keys).

- `GET /api/provider-kinds` — the kind catalog (form schema per kind).
- `GET /api/providers`, `PUT /api/providers`, `DELETE /api/providers/{id}` —
  instance CRUD. Secrets never leave the API; the DTO only exposes
  `secretsSet: {"key": true}`.
- An **agent** selects a provider instance (`providerInstanceId`); its
  `provider` field is then a **derived** kind id, kept in sync automatically —
  do not set `provider` directly when creating/updating an agent.
- These are managed through the Providers panel / provider CRUD tools, **not**
  through `get_settings`/`update_settings` — there is no `defaultProvider`,
  `minimaxKey`, `openrouterKey`, `zaiKey`, `deepseekKey`, or `anthropicKey`
  field on `settings.json` anymore.

### User profile (injected into CHAT turns so agents address the user correctly)
- `userName`, `userTimezone`, `userCity`, `userCountry`, `userNotes` — rendered as an
  "## About the user" block in the chat system prompt (`api.userContextBlock`), together
  with the reply-language directive. **Chat path only** — autonomous/scheduler/flow turns
  do not currently inject this block.

### Context
- `maxContextTokens` (min 500, default 12000), `keepRecentMsgs` (min 1, default 8).
- `contextBudgetCeil` (8000–2000000, default 262144 ≈ 256K) — hard cap on the model-aware transcript budget; the operative number for 1M-window models. Lowered from 512K to keep the live window in the context-rot gradient's high-precision zone; raise to keep more history verbatim (trades recall precision for raw history). `contextBudgetFraction` (0–1, default **0 = auto**) — share of the model's context window spendable on transcript. **0 selects a per-family adaptive share** (Opus/Sonnet 0.45, Haiku 0.40, MiniMax/DeepSeek/Gemini 0.35); a positive value pins a fixed manual share. Effective budget = clamp(window × fraction, maxContextTokens, ceil). Rationale: see `_Docs/17` §12.

### Turn recovery
- `reactiveCompact` (default true) — fold history + retry on context overflow.
- `maxTokenRetries` (0–10, default 3), `reactiveKeepRecent` (2–50, default 6).
- `maxOutputTokens` (default 0 = auto) — per-turn generation cap (`max_tokens`).
  0 resolves per model family (opus/sonnet/fable+minimax 32K, haiku 16K,
  deepseek/gemini 8K, unknown → provider 4096 fallback); a positive value (clamped
  256–512000) pins a fixed cap across all models. Env fallback when 0:
  `TIONHARNESS_MAX_OUTPUT_TOKENS`.

### Session debug journal (observability)
- `debugJournalEnabled` (default true) — write the parallel `debug.jsonl` stream per session (turn timings, per-call token spend, per-tool latency/size/errors, hook decisions, compaction/recovery). Off = no debug events written.
- `debugJournalCap` (default 5000, `0` = default) — newest events kept per session; older ones are pruned once the file passes cap + cap/4. Read it with the `read_session_debug` tool or `GET /api/sessions/{id}/debug`. See `_Docs/38-SESSION-DEBUG.md`.

### Autonomy
- The per-agent daily spend caps (`defaultDailyCallLimit`/`defaultDailyTokenLimit` +
  per-agent `dailyCallLimit`/`dailyTokenLimit` + enforcement) were **removed entirely**
  (2026-07-01) — agents are always unlimited. Only spend *tracking* remains (the Budget
  screen still shows usage).
- The autonomy pause is **per-workspace, not app-global** (2026-07-01): the app-wide
  `pauseAutonomy` setting was removed. Pause is now the workspace-settings field
  `pauseAutonomy` (`ws-settings.json`), toggled from the **Schedules** screen. It
  blocks only that workspace's scheduled calls. Not an app-settings key anymore.
- `autoTitleEnabled` (default true) — the only auto-title knob. The model and prompt
  come from the built-in **titler** system agent; the old `titleModel` /
  `titleProviderId` overrides were removed (2026-08-28).

### Context reset / handoff (see _Docs/35)
- `handoffAuto` (default false) — auto-handoff a near-limit session (autonomous turns only).
- `handoffPressure` (default 0.90) — context-fill ratio that triggers auto-handoff.
- `handoffMaxChain` (default 20) — max handoff chain length.
- `handoffWriteFile` (default false) — also write the handoff doc to a file.

### Persistent progress (see _Docs/36)
- `progressPersist` (default true) — persist the `todo_write` list to `<cwd>/.tionharness/progress.json`.
- `progressResume` (default true) — restore that list on a fresh session.

### Autonomous self-completion (see _Docs/05)
- `autonomousAutoContinue` (default true) — when an autonomous turn (scheduler/spawn/wake) ends with unfinished work (it left `todo_write` items open, or its last action was a lazy-tool activation whose tools only take effect next turn), automatically run a continuation turn so the work self-completes instead of stalling. Each continuation is history-aware and budget-gated; the loop stops when the work is done, a turn makes no tool progress, or the daily budget is hit.
- `autonomousAutoContinueMax` (default 3, 0 = default) — hard cap on auto-issued continuation turns per autonomous run.

### File freshness guard (Claude Code parity)
- `fileFreshnessGuard` (default true) — the built-in `Edit`/`Write` tools require a
  file to have been `Read` this session and to be unchanged since, before it may be
  edited or overwritten. An edit of an unread file errors "file has not been read
  yet"; an edit after an out-of-band change errors "file has been modified since it
  was last read". A brand-new `Write` needs no prior read. Session-scoped, native
  path only (claude-cli agents have their own equivalent). Disable to restore the
  old unchecked behaviour.

### MCP & gated tool capabilities (off by default — each expands power/cost)
- (The dead `mcpGatewayUrl` setting was removed entirely 2026-07-01 — it was never
  consumed by the runtime. MCP servers are managed in the "Araçlar & MCP" category.)
- `enableShell` — the built-in shell tool. On claude-cli agents it is bridged
  (PowerShell on Windows) and the CLI's native `Bash` is suppressed so commands
  route through TionHarness's shell.
- `enableSelfManage` — **REMOVED (2026-07-01).** The self-management suite is now
  ALWAYS built; visibility is per-tool (full / summary / name-only / hidden) on the
  "Araçlar" screen, defaulting to `hidden`. The settings field, its `TIONHARNESS_ENABLE_SELFMANAGE`
  env seed, and the `SelfManageEnabled` tunable were all deleted. See `_Docs/19`.
- `enableCliHooks` — pass PreToolUse/PostToolUse hooks to claude-cli agents via
  `--settings` (default true). Turn off to keep hooks native-only when a hook
  authored for TionHarness's shell misbehaves under the CLI's own hook runner.
- `claudeResume` — keep the claude-cli session warm across turns (default **true**).
  When on, each single-agent turn passes `--resume <id>` and sends only the new
  delta (not the full transcript), so the CLI reuses its server-side prompt cache
  (much cheaper). The resume id is tracked per session; warm mode delegates context
  management to the CLI, so TionHarness's own compaction is bypassed for that session.
  Effective only because the appended system prompt is now byte-stable (the volatile
  dynamic context rides in the message tail, not the cached system prefix).
- `claudePersistentSession` — keep ONE long-lived claude-cli process alive per
  session and feed turns over stdin (stream-json input) instead of spawning a fresh
  process each turn; warm turns ship only the new user message (default **true**).
  Supersedes `claudeResume` when on (when both are on, only the persistent process is
  used — the `--resume` delta path is disabled). Retains context in-process and reuses
  the prompt cache.
  > UI: these two booleans are edited as a SINGLE 3-way selector in Settings ▸
  > Yetenekler (`AppToolsPanel` `Segmented` "claude-cli cache/oturum modu"):
  > **Kalıcı süreç** (`persistent=true`), **--resume (delta)** (`persistent=false,
  > resume=true`), **Kapalı** (both false). The selector maps to the same booleans, so
  > the mutual-exclusion "both on → persistent wins" state is no longer reachable from
  > the UI. Note: `--resume (delta)` is the exact mode External Agent uses (respawn +
  > `resume: sessionId` per turn); the persistent process is a TionHarness-only addition.
- `claudeSysPromptFile` — how the appended claude-cli system prompt is delivered
  (default **false** = inline). Off: passed on the command line via
  `--append-system-prompt <text>` — simplest, no temp file. On: written to a temp file
  and passed via `--append-system-prompt-file <path>`, which sidesteps the Windows
  ~32 KB command-line limit (errno 206) for very large system prompts. Turn on only if
  a large prompt keeps the process from launching in inline mode.
- `run_subagent` (isolated subagent workers, agent→agent delegation) is ALWAYS
  installed (the `enableDelegation` master toggle was removed 2026-07-02); enable/
  disable it per-agent from the Tools screen. `delegationMaxDepth` (1–10, default 3)
  and `delegationMaxCalls` (1–100, default 8) remain as per-turn safety guards.
- `spawnMaxConcurrent` (1–128, default 16), `spawnQueueMax` (1–128, default 16),
  `spawnMaxPerTurn` (1–64, default 4).

### Working-directory guards
The built-in fs/shell tools are UNCONFINED (they may read, write and run on any
path; the permission mode is the safety boundary). These guards rein that in
for autonomous (no-human) turns; interactive chat is unaffected.
- `autonomousConfine` (default true) — on scheduler/spawn/flow turns, confine the
  fs tools to the session's working dir (reject absolute paths + `..` escapes) and
  block `git push`. Recommended: on. **Scope:** `shell` and `powershell` stay
  path-unrestricted, so a confined turn can still write anywhere via a shell
  command. `transform_data` resolves its input/output argument paths through the
  sandbox, but its arbitrary host-code script can bypass that restriction.
  Documented, not fixed: a real boundary needs OS-level isolation.
- `autonomousBootSeq` (default true) — on scheduler/spawn/flow/subagent turns,
  inject a short boot/verification-sequence reminder (orient → recall → select one
  task → verify the baseline → work → close the loop) into the system prompt. The
  full recipe is in the `tionharness-autonomous-ops` skill (§10). Costs a few tokens per
  headless turn; turn off to reclaim them. Interactive chat is unaffected.

### Workspace backups
Periodic, retention-bounded zip snapshots of every workspace's data dir.
- `backupEnabled` (default false) — master switch for the automatic schedule.
- `backupIntervalHours` (≥1, default 24) — hours between automatic runs; the first
  run happens one interval after the setting takes effect (no run on every restart).
- `backupRetain` (≥1, default 7) — newest archives kept per workspace; older pruned.
- `backupDir` (default "" → `<dataDir>/backups`) — absolute path of the backups root.
Live: changing these reconfigures the loop immediately. An on-demand run is also
available over `POST /api/backups/run` (not an agent tool).

### Diagnostics
- `logLevel` — `"info"` | `"debug"` | `"warn"` | `"error"`. **Not wired to the logger**
  (never read by `cmd/tionharness`) and removed from the Settings UI (2026-06-30); the Logs
  screen already filters by level + text, so it was redundant. Field kept in
  settings.json for backward compat only.

## Safety notes

- These tools change settings for the **whole application** (all workspaces).
  Confirm intent before flipping capability switches like `enableShell`.
- Out-of-range numbers are clamped, not rejected — read back the result to see
  the value that actually took effect.
- Invalid enum/format values (e.g. an unknown `theme`, a malformed `accent`, a
  bad `defaultPermissionMode`) are **rejected** with a clear error and nothing is
  changed — fix the value and retry.
- For the full conceptual overview of TionHarness, see the `tionharness-guide` skill.
