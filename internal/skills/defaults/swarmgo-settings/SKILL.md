---
name: "SwarmGo Settings"
description: "Every application-wide setting in SwarmGo (settings.json) — what each field does, its valid range/default — and how to read and change them live with the get_settings / update_settings tools."
when_to_use: "When you need to inspect or change SwarmGo's application settings: theme, providers, default model, context/memory budgets, autonomy, compaction, or the gated tool capabilities"
icon: "⚙️"
color: "#8b5cf6"
access: shared
auto_summary: false
---
# SwarmGo — Application Settings

SwarmGo keeps all application-wide configuration in a single JSON document,
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
update_settings → {"patch": {"pauseAutonomy": true}}
```

## Settings reference

### Appearance
- `theme` — `"dark"` | `"light"` | `"system"` (default `dark`).
- `accent` — hex color, e.g. `"#8b5cf6"`.
- `themePreset` — curated palette id (`""` = legacy theme+accent).
- `language` — `"tr"` | `"en"` (default `tr`).
- `desktopNotifications`, `keepAwake` — booleans (applied client-side).

### Providers & model
- `defaultProvider` — `"claude-cli"` | `"anthropic"` (default `claude-cli`).
- `defaultModel` — model id; `""` = the provider's own default.
- `defaultPermissionMode` — seeds new agents: `"read-only"` | `"ask"` | `"auto"`.
- `claudeCliPath` — path to the `claude` binary; `""` = auto-detect on PATH.
- `anthropicKey` — **write-only**; `""` clears. Read shows only `anthropicKeySet`.
- `minimaxKey` (write-only), `minimaxBaseUrl` — MiniMax (OpenAI-compatible).
- `openrouterKey` (write-only), `openrouterBaseUrl` — OpenRouter (OpenAI-compatible; one key, hundreds of models via `author/model-slug` ids).
- `oneMillionContext`, `extendedPromptCache` — Anthropic betas (anthropic only).
- Custom providers are managed separately (Providers panel / market), not patched here.

### User profile (injected so agents address the user correctly)
- `userName`, `userTimezone`, `userCity`, `userCountry`, `userNotes`.

### Context & memory
- `maxContextTokens` (min 500, default 12000), `keepRecentMsgs` (min 1, default 8).
- `recallTopN` (default 5), `recallMinScore` (0–1, default 0.05).
- `journalCap` (1–1000, default 50), `journalMaxLen` runes (64–65536, default 1024).
- `autoReflect` (default true), `autoReflectThreshold` (2–1000, default 30).

### Turn recovery
- `reactiveCompact` (default true) — fold history + retry on context overflow.
- `maxTokenRetries` (0–10, default 3), `reactiveKeepRecent` (2–50, default 6).

### Tool-output compaction
- System A (deterministic, free): `compactToolOutput` (default true),
  `compactMaxLines` (0–5000, default 200), `compactMaxBytes` (0–262144, default 12288).
- System B (LLM summary, opt-in): `compactLlmSummary` (default false),
  `compactLlmThreshold` bytes (default 8192), `compactModel` (`""` = title model).

### Budgets & autonomy
- `defaultDailyCallLimit`, `defaultDailyTokenLimit` — new-agent defaults (0 = unlimited).
- `pauseAutonomy` (global autonomy brake — pauses scheduled calls).
- `autoTitleEnabled` (default true), `titleModel` (`""` = agent's model).

### MCP & gated tool capabilities (off by default — each expands power/cost)
- `mcpGatewayUrl` — external MCP gateway URL.
- `enableShell` — the built-in shell tool. On claude-cli agents it is bridged
  (PowerShell on Windows) and the CLI's native `Bash` is suppressed so commands
  route through SwarmGo's shell.
- `enableSelfManage` — the self-management suite (create/edit/delete entities,
  **including these settings tools**).
- `enableCliHooks` — pass PreToolUse/PostToolUse hooks to claude-cli agents via
  `--settings` (default true). Turn off to keep hooks native-only when a hook
  authored for SwarmGo's shell misbehaves under the CLI's own hook runner.
- `enableDelegation` — `run_subagent` (isolated subagent workers, agent→agent
  delegation); `delegationMaxDepth` (1–10, default 3), `delegationMaxCalls`
  (1–100, default 8).
- `spawnMaxConcurrent` (1–128, default 16), `spawnMaxPerTurn` (1–64, default 4).

### Diagnostics
- `logLevel` — `"info"` | `"debug"` | `"warn"` | `"error"` (applied on restart).

## Safety notes

- These tools change settings for the **whole application** (all workspaces).
  Confirm intent before flipping capability switches like `enableShell`.
- Out-of-range numbers are clamped, not rejected — read back the result to see
  the value that actually took effect.
- Invalid enum/format values (e.g. an unknown `theme`, a malformed `accent`, a
  bad `defaultPermissionMode`) are **rejected** with a clear error and nothing is
  changed — fix the value and retry.
- For the full conceptual overview of SwarmGo, see the `swarmgo-guide` skill.
