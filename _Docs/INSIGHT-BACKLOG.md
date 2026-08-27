# TionHarness — Insight App-Fix Backlog

Auto-appended by the retrospective scanner (_Docs/60). Each entry carries a
stable `insight-sig` marker so re-scans never duplicate it.

## Flow execution fails at runtime when a node id is missing from the flow graph

- **Severity:** med
- **Occurrences:** 1
- **Evidence sessions:** SES160
- **File:** `internal/agent/runtime.go`

**Root cause:** The flow runner resolves node references (e.g. an agent node like "thanks") lazily during execution instead of validating that every referenced node exists when the flow is loaded/started. A dangling or typo'd node reference only surfaces as a mid-run flow_failure instead of being rejected upfront.

**Proposed fix:** Add a pre-run validation pass that walks the flow graph and confirms every referenced node id resolves to a defined node, rejecting the FlowRun before it starts (extending the same guardrail class as the recent 'reject unrunnable flows before creating a FlowRun' work) so this failure mode is caught at creation time rather than at execution time.

<!-- insight-sig:flow_failure:node_not_found:agent_node -->

## MCP tool routed through non-existent alias `get_flow`

- **Severity:** med
- **Occurrences:** 1
- **Evidence sessions:** SES158
- **File:** `internal/tools/registry.go`

**Root cause:** The agent attempted to call `get_flow` both as a bare tool name and as `mcp__tionharness_extended__get_flow`, but neither is registered in the tool registry — indicates either a missing tool registration/export in the MCP extended toolset or a stale tool name surfaced to the model (e.g. in a prompt/schema hint) that doesn't match what's actually wired up in internal/tools/registry.go.

**Proposed fix:** Either register a `get_flow` tool in the MCP extended server (if one was intended to exist) or remove/rename any reference to it in prompts, tool descriptions, or schema docs so the model never attempts to call a tool that isn't registered.

<!-- insight-sig:tool:get_flow|error:No such tool available -->

## Bash tool exposed to model but disabled in this execution context

- **Severity:** med
- **Occurrences:** 1
- **Evidence sessions:** SES158
- **File:** `internal/agent/toolsetup.go`

**Root cause:** The tool loop advertises `Bash` as an available tool to the model even though the current session/context has it disabled, causing the model to call it and receive a context-mismatch error instead of never seeing it as an option.

**Proposed fix:** Filter the tool list presented to the model based on which tools are actually enabled for the current context, rather than exposing disabled tools and rejecting them at call time.

<!-- insight-sig:tool:Bash|error:exists but is not enabled in this context -->

## `ask_user` guardrail times out with no fallback

- **Severity:** low
- **Occurrences:** 1
- **Evidence sessions:** SES158
- **File:** `internal/agent/toolloop.go`

**Root cause:** When the agent calls `ask_user` and no human responds within the timeout window, the tool call simply errors out with a generic timeout, leaving the agent without a defined recovery path (e.g. proceed with default, abort gracefully, or retry).

**Proposed fix:** Define explicit timeout-handling behavior for `ask_user` — either an app-level default response, a bounded retry/backoff, or a clear terminal state the agent can act on instead of a bare timeout error.

<!-- insight-sig:tool:ask_user|error:operation timed out -->

## transform_data reports "script exited 0 but did not write the output file" even though the script wrote the file successfully

- **Severity:** high
- **Occurrences:** 1
- **Evidence sessions:** SES2
- **File:** `internal/tools/builtin_transform.go` ⚠️ (not found in repo — pointer unverified)

**Root cause:** The transform_data builtin validates success by stat-ing the output_file path from the Go process's own filesystem/working directory after the script exits. The script itself runs in a different context (sandboxed/temp cwd, or a shell where /tmp resolves elsewhere on Windows), and the tool's post-run existence check races the child's buffered write/flush. Evidence: every failure carries the script's own stdout proving the write happened ("OK - size: 47535", "exists: True size: 47130", "OK new size: 47681") while the tool still declares the file missing. The tool therefore turns successful transforms into hard errors, and the agent retries with a new filename each time (/tmp/result.json, /tmp/c1.json, /tmp/c2.json, /tmp/w1.json, /tmp/c4.json), producing 9 consecutive guardrail warnings.

**Proposed fix:** Resolve output_file against the exact cwd/root handed to the child process and stat it there (not the server's cwd). Wait for child stdio close (not just exit) before stat-ing, and retry the stat briefly. If the script exits 0 and the file is absent, return the script's stdout/stderr as a soft result rather than a tool error, or accept stdout as the payload when the file is optional.

<!-- insight-sig:transform_data|script exited 0 but did not write the output file -->

## Edit/apply_patch never match on a file that a script can patch — likely line-ending/normalization mismatch between Read output and on-disk bytes

- **Severity:** high
- **Occurrences:** 1
- **Evidence sessions:** SES2
- **File:** `internal/tools/builtin_edit.go` ⚠️ (not found in repo — pointer unverified)

**Root cause:** Edit failed 4x and apply_patch failed with "hunk 1 does not match the file" on the same Windows/Unity C# file (WeeklyEventEditorModel.cs), while a transform_data script doing a raw string replace on the same file succeeded ("replaced 152 chars with 4159 chars"). That asymmetry points at the app, not the caller: the Read tool almost certainly normalizes CRLF→LF (and/or strips a BOM) when rendering content, but Edit/apply_patch match old_string against the raw on-disk bytes. Any old_string copied verbatim from Read can then never match a CRLF file, so no amount of "read the file again" advice fixes it.

**Proposed fix:** Make Edit/apply_patch match with the same normalization Read applies: detect the file's dominant line ending and BOM on load, normalize both haystack and needle for matching, then re-apply the original ending style when writing back. Add a regression test on a CRLF + BOM fixture.

<!-- insight-sig:Edit|old_string not found in file (CRLF/BOM-normalized read vs raw-byte match) -->

## activate_tools rejects MCP-namespaced tool names that are not mirrored into the on-demand catalog

- **Severity:** high
- **Occurrences:** 1
- **Evidence sessions:** SES41
- **File:** `internal/tools/registry.go`

**Root cause:** The on-demand tool catalog used by activate_tools is built only from builtin/registry tool definitions; tools exposed through the MCP pool (mcp__<server>__<tool>) are advertised to the model (or inferred by it from prior context) but never registered as activatable entries. The activation call therefore resolves nothing and returns 'unknown (not in the on-demand catalog)' for every mcp__* name, leaving the agent with no path to the MCP tools it was told about.

**Proposed fix:** When the MCP pool is scoped/attached for a session, register each discovered MCP tool into the same on-demand catalog that activate_tools resolves against (name, description, schema), and make activate_tools return a distinguishable error for 'server not connected' vs 'name does not exist' instead of one generic unknown-name message.

<!-- insight-sig:activate_tools:unknown-tool-not-in-on-demand-catalog:mcp-namespaced -->

## Bash tool is advertised in the prompt but disabled in the execution context, producing a dead-end tool call

- **Severity:** high
- **Occurrences:** 1
- **Evidence sessions:** SES41
- **File:** `internal/agent/toolsetup.go`

**Root cause:** Tool exposure and tool enablement are computed in two places that disagree: the agent's tool list/system prompt still describes Bash while the per-context (session/toolset) gate rejects it at call time with 'Bash exists but is not enabled in this context'. There is no guardrail that reconciles the advertised set with the enabled set, so the model burns a step on a call that can never succeed, and activate_tools offers no way to enable it.

**Proposed fix:** Derive the advertised tool list from the same enabled-set that the toolloop enforces (single source of truth in toolsetup), and if a disabled tool is called, return an actionable error naming the tool that IS available for the same capability (e.g. shell/exec alternative) rather than a bare 'not enabled'.

<!-- insight-sig:Bash:no-such-tool-available-not-enabled-in-this-context -->

## MCP tool transport drop mid-call loses the in-flight response with no reconnect/retry

- **Severity:** high
- **Occurrences:** 1
- **Evidence sessions:** SES45
- **File:** `internal/mcp/pool.go`

**Root cause:** The MCP client pool treats a transport disconnect during an outstanding tool call as a terminal error: the pending request is failed with "transport dropped mid-call; response was lost" and surfaced verbatim to the agent. There is no session re-establishment plus idempotent re-issue (or at minimum a bounded retry for read-only/wait-style calls), so a single stdio/pipe hiccup on a long-running MCP tool (here a `wait_until` poll that outlives the transport's idle window) permanently breaks the step instead of transparently recovering.

**Proposed fix:** In the MCP pool, wrap tool invocations in a supervised call: on transport EOF/close with in-flight requests, reconnect the client, re-initialize the session, and re-issue calls that are declared safe to retry (or expose a per-tool `retryable` policy). For genuinely non-idempotent calls, return a structured, typed error the toolloop can classify as retryable rather than a free-text string. Also add a keepalive/ping and an idle-timeout longer than the longest permitted tool timeout so `wait_until`-style long polls don't outlive the transport.

<!-- insight-sig:mcp:transport_dropped_mid_call:response_lost -->

## Agent calls Bash but the tool is not enabled in the session's tool context

- **Severity:** med
- **Occurrences:** 2
- **Evidence sessions:** SES50, SES40
- **File:** `internal/providers/claudecli.go`

**Root cause:** TionHarness advertised or allowed the model to attempt the Bash tool while the session's actual enabled tool set for this context did not include it, so every Bash invocation is rejected by the runtime with 'not enabled in this context'. This is a provisioning mismatch: the tool allowlist / capability set handed to the agent for this session kind is inconsistent with what the model believes it can call. It repeated (2x), so the model has no working shell fallback and burns steps retrying the same unavailable tool.

**Proposed fix:** Make the enabled tool set authoritative: either register/enable Bash for session kinds that need shell access, or strip Bash from the model's advertised toolset for contexts where it is intentionally disabled so it is never offered. On a 'tool not enabled' rejection, surface a guardrail that redirects the model to the equivalent available tool instead of letting it retry the same disabled tool.

<!-- insight-sig:Bash|no-such-tool-available-not-enabled-in-context -->

## activate_tools rejects tool names the agent was told exist (catalog/prompt drift)

- **Severity:** med
- **Occurrences:** 2
- **Evidence sessions:** SES33, SES32
- **File:** `internal/tools/registry.go`

**Root cause:** The on-demand tool catalog exposed to activate_tools does not contain every tool name the model is led to believe is activatable. The agent requested `create_skill` and `skill_validate`; both were reported as "unknown (not in the on-demand catalog)", so the call was a complete no-op. This is a registry/prompt mismatch inside TionHarness: either those tools are registered under different names (or only in a different scope/session kind) while the prompt or an earlier hint still advertises them, or they were removed from the deferred catalog without updating the catalog description the model sees. The failure is silent-ish — activate_tools returns success-shaped text ("no new tools activated") rather than an error, so the loop can proceed without the capability it just asked for.

**Proposed fix:** Make the on-demand catalog the single source of truth: generate the activatable-tool list handed to the model directly from the registry's deferred set (so an unregistered name can never be advertised), and have activate_tools return a hard error — not a benign "no new tools activated" line — when *every* requested name is unknown, including a nearest-match suggestion list from the catalog. Additionally, alias or register skill-authoring tools (create_skill / skill_validate) if they are intended to be activatable in this session kind.

<!-- insight-sig:activate_tools:unknown_tool_not_in_on_demand_catalog -->

## Agent calls `Bash` but the tool is not enabled in its context — tool advertisement/registry drift

- **Severity:** med
- **Occurrences:** 2
- **Evidence sessions:** SES31, SES34
- **File:** `internal/tools/registry.go`

**Root cause:** The agent's prompt/tool guidance still exposes `Bash` as a usable tool name while the runtime tool registry for this session kind does not enable it. The model therefore emits a `Bash` tool_use that the loop rejects with `No such tool available: Bash. Bash exists but is not enabled in this context.` Nothing in the app reconciles the advertised tool list with the actually-enabled set before the turn, so the failure is deterministic rather than transient, and it burns a full tool round-trip plus a repair turn every time it happens.

**Proposed fix:** Make the enabled-tool set the single source of truth: build the per-session tool guidance from `registry`'s actually-enabled tools (rather than a static list), and when a disabled-but-known tool is invoked, have the tool loop rewrite the error into an actionable alias hint pointing at the enabled equivalent (e.g. the shell/exec tool that IS enabled) instead of a bare rejection.

<!-- insight-sig:tool:Bash|error:no_such_tool_available_not_enabled_in_context -->

## Model calls shell tools (PowerShell/Bash) that are not enabled in the session's toolset

- **Severity:** med
- **Occurrences:** 1
- **Evidence sessions:** SES1
- **File:** `internal/agent/toolsetup.go`

**Root cause:** The session's system prompt / tool guidance still advertises shell execution (PowerShell, Bash) while the actual per-session tool registry built at setup time does not register them for this session kind. The agent therefore emits tool_use blocks for tools the runtime rejects with 'No such tool available', burning turns until it guesses an allowed alternative. The mismatch is between the prompt/tool-description surface and the enabled-tool set, not a user or transient error.

**Proposed fix:** Make the system prompt and tool catalog derive from the same source of truth as the registered toolset: when building a session, generate the tool list (and any prompt text mentioning PowerShell/Bash) from the registry actually installed for that session kind, so disabled shell tools are never mentioned. Additionally, on a 'no such tool available' rejection, return the concrete list of enabled tool names in the error payload so the model can self-correct in one step instead of retrying the same unavailable tool.

<!-- insight-sig:tool_not_available:shell_exec:no_such_tool_available -->

## Loop guardrail only warns — it never escalates, so a broken tool can burn 9+ identical failures in one turn

- **Severity:** med
- **Occurrences:** 1
- **Evidence sessions:** SES2
- **File:** `internal/agent/toolloop.go`

**Root cause:** The guardrail fires at failure 3 and then simply repeats the same warning text at 4, 5, 6, 7, 8, 9 with an incrementing counter. It has no escalation ladder: no hard block on the offending tool, no forced tool switch, no turn abort. Both failure modes in this session (Edit, transform_data) show the counter climbing while the agent keeps issuing near-identical calls, so the guardrail detected the loop correctly but did nothing to stop it.

**Proposed fix:** Escalate by threshold: warn at 3, at 5 disable that tool for the remainder of the turn and return a terminal error instructing the agent to use a different approach, at 7 halt the turn. Also suppress the loop counter when consecutive calls differ only in a volatile arg (e.g. output filename) — that pattern should escalate faster, not reset the detector.

<!-- insight-sig:guardrail|repeated warn, same tool, counter increments without escalation -->

## Agent calls Bash in a session kind where Bash is not enabled (tool-scope vs. prompt mismatch)

- **Severity:** med
- **Occurrences:** 1
- **Evidence sessions:** SES8
- **File:** `internal/agent/toolsetup.go`

**Root cause:** TionHarness scopes the tool set per session kind (the Board Planner session runs without shell access), but the model still emits a Bash tool call. The advertised toolset / system prompt the runtime hands to the provider is not consistent with the tools actually enabled for that session kind, so the provider rejects the call with 'Bash exists but is not enabled in this context'. The agent burns a turn on a call that the runtime could have known was impossible.

**Proposed fix:** Make the enabled tool list the single source of truth: build the provider tool schema and the system-prompt tool description from the same scoped registry used for session kind, and drop any mention of disabled tools. Additionally, intercept calls to known-but-disabled tools in the tool loop and return a short, actionable message naming the allowed alternatives instead of surfacing the raw provider rejection.

<!-- insight-sig:Bash|tool_not_enabled_in_context -->

## Bash tool rejects `run_in_background` at call time instead of hiding it from the schema

- **Severity:** med
- **Occurrences:** 1
- **Evidence sessions:** SES9
- **File:** `internal/tools/registry.go`

**Root cause:** The Bash tool's advertised JSON schema still exposes a `run_in_background` parameter even in execution contexts where background/detached execution is not wired up (no task tracker / no re-invoke channel for that session kind). The model therefore emits a schema-valid call that the executor rejects at runtime with a plain-text refusal, burning a full tool round-trip. The capability check lives in the handler, not in the schema that is sent to the provider.

**Proposed fix:** Make the Bash tool schema context-dependent: when the session/runtime cannot host background tasks, omit `run_in_background` from the emitted tool definition entirely (registry-level schema filtering) rather than accepting it and failing. Alternatively, degrade gracefully — run in the foreground and note it in the tool result instead of returning an error.

<!-- insight-sig:Bash:run_in_background_not_available -->

## create_artifact fails on absolute host paths produced by MCP servers outside the session workspace

- **Severity:** med
- **Occurrences:** 1
- **Evidence sessions:** SES10
- **File:** `internal/tools/builtin_artifact.go`

**Root cause:** create_artifact takes a source path and stats it directly on the host filesystem with no workspace-relative resolution and no pre-flight discovery. Files written by MCP servers (here, playwright-mcp's `.playwright-mcp/` output dir) land under the MCP server's own cwd, not the session workspace root, so the path the agent reconstructs does not exist. The tool then fails with a bare "source file not found" that gives the agent nothing to correct with — no candidate paths, no indication of which roots are searchable — so the agent cannot self-repair and the artifact is silently lost.

**Proposed fix:** Resolve the source path against a set of known roots (session workspace, MCP server cwds, configured output dirs) before failing, and on a miss return an actionable error listing the roots searched plus any near-match filenames found under them. Longer term, have MCP tool results that produce files register their absolute output paths in session state so create_artifact can look them up by name instead of relying on the model to reconstruct a path.

<!-- insight-sig:create_artifact:save-artifact-source-file-not-found -->

## activate_tools no-ops when the model asks for tools that exist in the product but are absent from the on-demand catalog

- **Severity:** med
- **Occurrences:** 1
- **Evidence sessions:** SES30
- **File:** `internal/tools/registry.go`

**Root cause:** The on-demand tool catalog consulted by activate_tools is a separate registry from the tools the agent is told about elsewhere (skill-related tools like create_skill / skill_validate are referenced in prompt/skill guidance but never registered as activatable entries). When the model requests them, activate_tools finds no catalog match, activates nothing, and returns a soft informational message rather than an actionable error — so the agent believes activation was attempted and proceeds without the capability, or loops re-requesting the same names.

**Proposed fix:** Make the on-demand catalog the single source of truth: register every non-always-on tool (including the skill tools) in the catalog at startup, and add a startup assertion/test that every tool name mentioned in system-prompt/skill guidance resolves in the catalog. Separately, have activate_tools distinguish the three outcomes explicitly — 'already active', 'activated', 'unknown name' — and return an error (with the list of valid catalog names) when every requested name is unknown, instead of a success-shaped no-op.

<!-- insight-sig:activate_tools:unknown-tools-not-in-on-demand-catalog -->

## `activate_tools` accepts unknown tool names and no-ops silently instead of failing with a valid-name list

- **Severity:** med
- **Occurrences:** 1
- **Evidence sessions:** SES34
- **File:** `internal/tools/registry.go`

**Root cause:** `activate_tools` was called with `create_skill` and `skill_validate`, neither of which exists in the on-demand catalog. The tool returns `no new tools activated (already active or none valid)` — it merges the "already active" and "name does not exist" cases into one message and does not enumerate what the agent could have activated. The model has no grounded catalog to draw from, so it invents plausible tool names; the app then gives it no signal to correct with, which invites repeat attempts in the same session.

**Proposed fix:** Split the response paths in the `activate_tools` builtin: on an unknown name, return an explicit error listing the valid on-demand catalog names (optionally with the nearest match by edit distance), and distinguish it from the benign "already active" case. Ideally also inject the catalog names into the tool's JSON schema as an enum so the model cannot emit an out-of-catalog name in the first place.

<!-- insight-sig:tool:activate_tools|error:unknown_tools_not_in_on_demand_catalog -->

## `activate_tools` accepts names that are not in the on-demand catalog and silently no-ops

- **Severity:** med
- **Occurrences:** 1
- **Evidence sessions:** SES31
- **File:** `internal/tools/registry.go`

**Root cause:** The model is told (or guesses) that skill-related tools such as `create_skill` and `skill_validate` are activatable, but they are not registered in the on-demand catalog. `activate_tools` validates the names only after the call, returning "no new tools activated ... unknown" — so the agent gets neither the tool nor an actionable alternative, and the catalog/prompt drift is invisible until runtime.

**Proposed fix:** Have `activate_tools` return the valid catalog (or nearest-name suggestions) in its error payload, and generate the activatable-tool list injected into the prompt from the catalog itself so unknown names can never be suggested to the model.

<!-- insight-sig:tool:activate_tools|error:unknown_tool_not_in_on_demand_catalog -->

## activate_tools silently no-ops when the model requests catalog names that don't exist (create_skill, skill_validate)

- **Severity:** med
- **Occurrences:** 1
- **Evidence sessions:** SES29
- **File:** `internal/tools/registry.go`

**Root cause:** The on-demand tool catalog exposed to the model advertises skill-related capabilities (or the system prompt / skills documentation names them) under identifiers that are not registered in the activation catalog. activate_tools resolves the requested names, finds no match, and returns a soft 'no new tools activated … unknown' message instead of a hard error or a correction, so the model has no actionable signal and re-issues the identical call — the same pair of unknown names appears twice in one session. The catalog and the names the model is told about are not derived from a single source of truth.

**Proposed fix:** Derive the on-demand catalog and the names surfaced to the model from the same registry map so an advertised capability cannot be unregistered. In activate_tools, when every requested name is unknown, return an error result (not a success message) that lists the closest valid catalog names, so the model corrects instead of repeating the call. Add a startup assertion that every tool name referenced in prompt/skill documentation resolves in the registry.

<!-- insight-sig:activate_tools|no_new_tools_activated|unknown_not_in_on_demand_catalog -->

## Model calls Bash, which exists in the provider but is not enabled in the TionHarness tool context

- **Severity:** med
- **Occurrences:** 1
- **Evidence sessions:** SES29
- **File:** `internal/agent/toolsetup.go`

**Root cause:** The provider-side tool allowlist for this session excludes Bash while the model's context still leads it to believe Bash is callable — the harness returns 'Bash exists but is not enabled in this context'. TionHarness's tool setup builds the enabled set without ensuring the model-visible tool list matches it, so the model wastes a turn on a tool that was never wired in. This is a wiring/guardrail gap in tool setup, not a transient error.

**Proposed fix:** Make the enabled tool set the only thing the model sees: when constructing the session, filter the advertised tool list to exactly the allowlisted set, and when a disabled-but-existing tool is called, surface a repair message naming the enabled equivalent (e.g. the shell/exec tool actually wired in) instead of a bare 'not enabled'. Log a warning at setup when the advertised list and the enabled list diverge.

<!-- insight-sig:Bash|no_such_tool_available|exists_but_not_enabled_in_this_context -->

## Model calls `Bash` in a session where the tool is registered but not enabled for the active scope

- **Severity:** med
- **Occurrences:** 1
- **Evidence sessions:** SES36
- **File:** `internal/agent/toolsetup.go`

**Root cause:** The toolset assembled for the session (program/workspace-scoped, e.g. the Unity 'CityCleaner' program) excludes Bash, but the model still emits a Bash tool_use — meaning the prompt/tool advertisement and the actually-enabled tool list are built from different sources. TionHarness's tool registry knows Bash exists and answers the call with 'exists but is not enabled in this context' instead of either (a) never advertising it, or (b) transparently mapping it to the enabled shell tool. The result is a wasted turn and a hard error step for a tool the app itself half-offers.

**Proposed fix:** Make the enabled-tool set the single source of truth: build the model-facing tool schema list in `toolsetup.go` from the same filtered registry that `toolloop.go` dispatches against, so a disabled tool is never nameable. Additionally, in the registry's unknown/disabled-tool path, return a repairable error that names the enabled equivalent (e.g. 'use PowerShell/Shell instead of Bash') rather than a generic not-enabled message, so the model self-corrects in one hop instead of erroring out.

<!-- insight-sig:tool:Bash|error:no-such-tool-available-not-enabled-in-this-context -->

## Glob/ripgrep repeatedly times out (20s) on large repos with no path narrowing or backoff — agent retries the same failing search 5×

- **Severity:** med
- **Occurrences:** 1
- **Evidence sessions:** SES33
- **File:** `internal/agent/toolloop.go`

**Root cause:** TionHarness surfaces the ripgrep timeout as a plain tool error and lets the model re-issue the identical Glob call. There is no app-side guardrail that (a) scopes searches away from vendor/build directories (Unity `Library/`, `Temp/`, `obj/`) or (b) blocks/rewrites a repeat of an already-timed-out pattern. The `cli_warn: Glob` guardrail fires but is advisory only, so the loop burns 5 identical failures before the agent gives up.

**Proposed fix:** In the tool loop, track (tool, normalized args) for timed-out searches and fail-fast on an exact repeat with a directive error ("this pattern already timed out; narrow the path or use a more specific glob"). Additionally inject default ignore globs for known heavy build/vendor dirs when the workspace root looks like a Unity/node/Go project, and make the ripgrep timeout configurable per workspace instead of a hard 20s.

<!-- insight-sig:Glob:ripgrep_search_timed_out_after_Ns -->

## Agent invokes Bash although the session's allowed-tools set excludes it

- **Severity:** med
- **Occurrences:** 1
- **Evidence sessions:** SES37
- **File:** `internal/agent/toolsetup.go`

**Root cause:** TionHarness builds the provider tool allowlist (toolsetup) independently of the system prompt / agent instructions handed to the model. When a workspace profile (here a Unity project session) is created without Bash in its enabled tool set, the prompt still describes shell usage, so the model calls `Bash` and the CLI rejects it with "exists but is not enabled in this context". The wasted turn is an app-side contract mismatch, not a model or user error.

**Proposed fix:** Derive the tool documentation in the system prompt from the same enabled-tool registry that is passed to the provider, so a disabled tool is never advertised. Additionally, when a tool call names a tool that exists in the registry but is disabled for the session, intercept it in the tool loop and return a deterministic 'tool disabled, use X instead' repair message listing the actually-enabled alternatives, rather than letting the provider-level error consume a step.

<!-- insight-sig:tool=Bash|error=no_such_tool_available_not_enabled_in_context -->

## Model is offered tools that are not enabled in the active context (Bash)

- **Severity:** med
- **Occurrences:** 1
- **Evidence sessions:** SES45
- **File:** `internal/agent/toolsetup.go`

**Root cause:** The tool registry/system-prompt assembly advertises a tool name to the model that the runtime's active toolset for this session kind does not actually expose. The model calls `Bash`, and the loop rejects it with "Bash exists but is not enabled in this context." This is a registry/prompt mismatch in TionHarness's tool setup, not a model mistake — the only reason the model knows the name is that the app leaked it.

**Proposed fix:** Make the advertised tool list and the executable tool list derive from a single source of truth in tool setup: filter the prompt/tool-schema list by the same context predicate used at dispatch time. If a disabled tool must remain mentioned, the rejection should name the enabled equivalent explicitly instead of the generic "use one of the available tools".

<!-- insight-sig:toolloop:no_such_tool_available:tool_exists_but_not_enabled -->

## activate_tools silently no-ops on names outside the on-demand catalog and gives no valid-name feedback

- **Severity:** med
- **Occurrences:** 1
- **Evidence sessions:** SES45
- **File:** `internal/tools/registry.go`

**Root cause:** `activate_tools` accepts arbitrary strings, reports "no new tools activated" and lists the unknown names, but does not distinguish "already active" from "never existed" and returns no catalog of what *could* be activated. MCP-server tool names (`mcp__<server>__<tool>`) appear callable to the model but are absent from the on-demand catalog, so the model burns steps re-attempting activation with no way to correct itself.

**Proposed fix:** Either register MCP server tools into the on-demand catalog under their canonical `mcp__server__tool` names, or make `activate_tools` return a structured error that separates already-active / unknown / not-activatable names and includes the list of activatable names (or nearest matches) so the model can self-correct in one turn.

<!-- insight-sig:tool:activate_tools:unknown_not_in_on_demand_catalog -->

## MCP resource handles are handed to the model without content capture, so later retrieval fails after TTL/eviction

- **Severity:** med
- **Occurrences:** 1
- **Evidence sessions:** SES45
- **File:** `internal/agent/toolloop.go`

**Root cause:** TionHarness passes MCP resource links (`<server>://resource/<id>`) through to the model as opaque handles and only dereferences them when the model later calls `*_retrieve`. Since the producing server may expire or evict the artifact between turns, the retrieve fails with ARTIFACT.NOT_FOUND (`retryable: false`) and the step is unrecoverable — the app has no cached copy and no way to regenerate the handle.

**Proposed fix:** When an MCP tool result contains a resource link, resolve and cache the artifact bytes (or a workspace-local copy) at result-ingest time and rewrite the handle to point at the local copy. Fall back to a clear, actionable error ("artifact expired — re-run the producing tool") that the toolloop can turn into a repair step, rather than surfacing the raw NOT_FOUND.

<!-- insight-sig:mcp:artifact_retrieve:not_found_expired_or_evicted -->

## Agent repeatedly calls `Bash` even though it is not in the enabled tool set for the session

- **Severity:** med
- **Occurrences:** 1
- **Evidence sessions:** SES40
- **File:** `internal/agent/toolsetup.go`

**Root cause:** The tool set handed to the provider for this session does not include Bash, but nothing tells the model that. The model still emits a Bash tool_use (it is a well-known default tool name), and the provider rejects it with "No such tool available: Bash. Bash exists but is not enabled in this context." The call is wasted twice in a row, which means the rejection is fed back as a plain tool error with no corrective steering — TionHarness neither filters the disallowed call before dispatch nor injects an explicit "available tools" statement into the system prompt when the enabled set is a restricted subset. This is a tool-registry/prompt mismatch inside TionHarness, not a transient or user error.

**Proposed fix:** In the tool setup path, (1) render the actual enabled tool names into the system prompt when the session runs with a restricted tool set, so the model does not guess at defaults like Bash/Write; and (2) in the tool loop, intercept a tool_use whose name is not in the enabled registry and return a structured repair message that names the closest enabled alternative (e.g. PowerShell/Shell) instead of forwarding the provider's generic rejection. Optionally alias well-known names (Bash → the enabled shell tool) rather than hard-failing.

<!-- insight-sig:tool=Bash | error=no_such_tool_available:tool_not_enabled_in_context -->

## transform_data accepts a call with no output_file and only fails at execution time ("output_file is required")

- **Severity:** low
- **Occurrences:** 1
- **Evidence sessions:** SES2
- **File:** `internal/tools/registry.go`

**Root cause:** output_file is enforced imperatively inside the tool handler rather than being declared as a required property in the tool's JSON schema. The model therefore has no schema-level signal that the argument is mandatory, and the omission surfaces as a runtime tool error that counts against the loop guardrail.

**Proposed fix:** Mark output_file as required in the transform_data schema (and validate args against the schema before dispatch) so the omission is caught at validation time with a schema-derived message instead of a runtime failure.

<!-- insight-sig:transform_data|output_file is required -->

## Edit tool hard-fails on stale read instead of auto-refreshing the file snapshot

- **Severity:** low
- **Occurrences:** 1
- **Evidence sessions:** SES9
- **File:** `internal/tools/registry.go`

**Root cause:** The Edit tool enforces a read-before-write freshness check against a cached mtime/content snapshot. Any out-of-band write between the Read and the Edit — including writes made by TionHarness's own formatter/linter hooks or by a concurrent agent in the same workspace — invalidates the snapshot and the edit is rejected, forcing the agent to re-Read and re-emit the identical edit. The guardrail does not distinguish "the file changed in a way that affects my `old_string` anchor" from "the file changed elsewhere", so benign concurrent edits produce failures.

**Proposed fix:** Downgrade the check from mtime/snapshot equality to anchor validity: on a stale snapshot, re-read the file and retry the edit automatically if `old_string` still matches exactly once; only fail if the anchor is now missing or ambiguous. Also exclude TionHarness's own post-edit hook writes from invalidating the snapshot by refreshing the cached content after the hook runs.

<!-- insight-sig:Edit:file_modified_since_read -->

## todo_write update mode fails when no checklist exists instead of creating one

- **Severity:** low
- **Occurrences:** 1
- **Evidence sessions:** SES10
- **File:** `internal/tools/builtin_todo.go`

**Root cause:** The todo_write tool distinguishes an implicit "update" call from a "create" call by whether a checklist already exists in session state, but the tool schema does not force the model to pass the full todos array on first use. When the agent issues a partial/patch-style todo_write before any checklist has been seeded, the handler rejects it with a self-referential error instead of just seeding the checklist from the provided items. The tool is idempotent in principle (a full todos array is authoritative either way), so the create/update split is an artificial state dependency that only produces a wasted turn.

**Proposed fix:** Make todo_write upsert: if no checklist exists for the session and the call carries a non-empty todos array, create it rather than erroring. If the call is genuinely partial (no todos array), keep the error but make the schema require `todos` unconditionally so the model cannot construct the failing call in the first place.

<!-- insight-sig:todo_write:no-existing-checklist-to-update -->

## ask_user aracı yanıtsız kaldığında zaman aşımıyla turu askıya alıyor

- **Severity:** med
- **Occurrences:** 1
- **Evidence sessions:** SES182
- **File:** `internal/api/autonomous_interaction.go`

**Root cause:** TionHarness'in interaction MCP aracı olan ask_user, kullanıcıdan yanıt gelene kadar bloklanıyor. Otonom/gözetimsiz çalışan bir oturumda soruyu yanıtlayacak bir kullanıcı olmadığında istek varsayılan zaman aşımına kadar bekliyor ve ardından 'operation timed out' ile düşüyor. Bu bir kullanıcı/ortam hatası değil; aracın yanıtsız durumu zarifçe ele alacak bir guardrail (otomatik iptal, varsayılan cevap veya turu devam ettiren fallback) içermemesinden kaynaklanan uygulama seviyesinde bir eksiklik.

**Proposed fix:** autonomous_interaction akışında ask_user için açık bir zaman aşımı + fallback guardrail'i ekle: süre dolduğunda istegi iptal edip ajana yapılandırılmış bir 'no_response' sonucu döndür (hata fırlatmak yerine), böylece tur bloklanmadan devam etsin. Otonom modda ask_user'ı ya devre dışı bırak ya da varsayılan/timeout-safe bir yanıtla besle.

<!-- insight-sig:ask_user:operation_timed_out -->

## insight_* araçları on-demand katalogda kayıtlı olmadığı için activate_tools ile etkinleştirilemiyor

- **Severity:** med
- **Occurrences:** 1
- **Evidence sessions:** SES180
- **File:** `internal/agent/toolsetup.go`

**Root cause:** Ajan, insight akışını sürdürmek için insight_scan, insight_list_findings ve insight_apply_finding araçlarını activate_tools ile etkinleştirmeye çalışıyor; ancak bu araçlar on-demand (talep üzerine etkinleştirilebilir) araç kataloğuna hiç eklenmemiş. Katalogda tanımlı olmadıkları için activate_tools 'unknown (not in the on-demand catalog)' diyerek hiçbir yeni araç etkinleştirmiyor ve insight adımı sessizce ilerleyemiyor. Bu, kullanıcı/geçici bir hata değil, araç kayıt tablosundaki bir eksiklik — uygulama seviyesinde bir kayıt/şema hatası.

**Proposed fix:** insight_scan, insight_list_findings ve insight_apply_finding araçlarını on-demand araç kataloğuna kaydet (activate_tools tarafından çözümlenebilir hale getir). Alternatif olarak bu araçları insight bağlamında baştan aktif (always-on) kabul et; her iki durumda da katalog ile araç tanımları arasındaki isim eşlemesini doğrulayan bir başlangıç kontrolü ekle ki katalogda olmayan bir araç etkinleştirme isteği erken ve açık şekilde raporlansın.

<!-- insight-sig:activate_tools|unknown tool not in on-demand catalog|insight_scan,insight_list_findings,insight_apply_finding -->

## Flow çalıştırması, var olmayan agent node referansı yüzünden ancak run sırasında düşüyor

- **Severity:** med
- **Occurrences:** 1
- **Evidence sessions:** SES160
- **File:** `internal/orchestration/engine.go`

**Root cause:** Flow grafiği kaydedilirken veya run başlatılırken node referansları doğrulanmıyor. Motor node'ları sırayla yürütürken hedef agent node'unu (`"thanks"`) arıyor, bulamayınca run'ı `flow_failure: node ... not found` ile iptal ediyor. Yani eksik/yeniden adlandırılmış node referansı bir ön doğrulama (pre-flight validation) guardrail'i olmadığı için yalnızca çalışma anında, kısmen ilerlemiş bir run'ın ortasında yakalanıyor; kullanıcı kaybedilen adımları ve harcanan token'ları geri alamıyor.

**Proposed fix:** Flow kaydetme (`SaveFlow`) ve run başlatma yollarına bir graf doğrulama adımı ekle: tüm edge hedeflerinin, start/end bağlantılarının ve agent node'larının gerçekten var olan node id'lerine/agent kayıtlarına çözümlendiğini kontrol et; çözümlenmeyenleri run başlamadan önce tek seferde hata listesi olarak döndür. Ayrıca editörde (FlowCanvas/NodeInspector) kırık referansları görsel olarak işaretle ki hata çalıştırmadan önce görülsün.

<!-- insight-sig:flow_run:node_ref_not_found -->

## Windows oturumunda shell aracı yanlışlıkla `/bin/bash` çalıştırıyor

- **Severity:** high
- **Occurrences:** 1
- **Evidence sessions:** SES1110
- **File:** `internal/tools/shell.go` ⚠️ (not found in repo — pointer unverified)

**Root cause:** TionHarness, Windows çalışma alanı ve Windows biçimli Go yolu mevcut olmasına rağmen `shell` komutlarını Bash ortamına yönlendiriyor. Bu sağlayıcı/işletim sistemi uyumsuzluğu PATH üzerindeki Windows araçlarını görünmez kılıyor ve `C:\Program Files\...` yollarının Bash tarafından komut olarak yanlış ayrıştırılmasına neden oluyor.

**Proposed fix:** Shell yürütücüsünde işletim sistemini algılayıp Windows için varsayılan olarak PowerShell kullanın; komut, PATH ve boşluk içeren executable yollarını seçilen shell'e uygun biçimde oluşturup alıntılayın. Bash yalnızca açıkça seçildiğinde veya doğrulanmış bir WSL ortamında kullanılmalı.

<!-- insight-sig:shell:/bin/bash on Windows causes command not found for host executable -->

## Windows derlemesi tanımsız syscall sabiti nedeniyle başarısız oluyor

- **Severity:** high
- **Occurrences:** 1
- **Evidence sessions:** SES1108
- **File:** `internal/providers/process_alive_windows.go`

**Root cause:** `internal/providers/process_alive_windows.go`, Go'nun `syscall` paketinde bulunmayan `syscall.ERROR_INVALID_PARAMETER` tanımlayıcısına başvuruyor; bu nedenle TionHarness Windows hedefinde derlenemiyor.

**Proposed fix:** Windows hata kodunu desteklenen `golang.org/x/sys/windows` sabitiyle kullanın veya yerel bir `syscall.Errno(87)` sabiti tanımlayın; ardından Windows hedefli derleme testi ekleyin.

<!-- insight-sig:shell: go-build undefined: syscall.ERROR_INVALID_PARAMETER -->

## Windows oturumundaki shell komutları yanlışlıkla Bash ile çalıştırılıyor

- **Severity:** high
- **Occurrences:** 1
- **Evidence sessions:** SES1306
- **File:** `internal/tools/shell.go` ⚠️ (not found in repo — pointer unverified)

**Root cause:** TionHarness shell yürütücüsü, Windows çalışma ortamı ve Windows biçimli çalıştırılabilir yollarına rağmen komutları `/bin/bash` üzerinden başlatıyor. Bu sağlayıcı-kabuk uyuşmazlığı geçerli `C:\Program Files\...` yollarının ve PowerShell sözdiziminin Bash tarafından yanlış yorumlanmasına neden oluyor.

**Proposed fix:** Shell yürütücüsünde işletim sistemi ve oturum kabuğunu açıkça belirleyip Windows'ta varsayılan olarak PowerShell çalıştırın; komut oluşturma ve yol alıntılama mantığını seçilen kabuğa göre uygulayın. Windows bağlamında `/bin/bash` seçilmesini engelleyen bir guardrail ve platformlar arası yürütme testleri ekleyin.

<!-- insight-sig:shell:/bin/bash executes Windows command and cannot resolve Windows executable paths -->

## Ertelenmiş orkestrasyon araçları şemaları yüklenmeden çağrılabilir olarak sunuluyor

- **Severity:** high
- **Occurrences:** 1
- **Evidence sessions:** SES1269
- **File:** `internal/tools/registry.go`

**Root cause:** TionHarness araç kayıt/çağrı katmanı, deferred durumundaki araç adlarını modele açıyor ancak çağrıdan önce şema çözümlemesini tamamlamıyor. Bu nedenle akış, ajan ve görev yönetimi araçları kalıcı biçimde çağrılamıyor.

**Proposed fix:** Deferred araç çağrılarını yürütmeden önce şemayı otomatik yükleyip doğrulayan bir çözümleme adımı ekleyin; şema yüklenene kadar aracı çağrılabilir araç listesine dahil etmeyin ve eşzamanlı yüklemeleri tekilleştirin.

<!-- insight-sig:deferred_tool_call:tool "<tool>" is deferred and cannot be called before its schema is available -->

## Deferred araçlar şeması yüklenmeden autonomous modda çağrılıyor

- **Severity:** high
- **Occurrences:** 1
- **Evidence sessions:** SES1269
- **File:** `internal/providers/claudecli.go or internal/agent/ (tool dispatch/autonomous runner)` ⚠️ (not found in repo — pointer unverified)

**Root cause:** TionHarness autonomous ajanlar için deferred araçların şemalarını otomatik olarak ToolSearch ile yüklemeyen bir mekanizmaya sahip değildir. Ajon deferred araçları doğrudan çağırmaya çalışınca sistem çağrıyı reddediyor ama ajanı ToolSearch çağırması için yeniden yönlendiremediğinden, başarısız çağrı döngüsüne giriyor.

**Proposed fix:** Autonomous agent runner, çağrılan araç deferred ise ToolSearch ile şemayı yüklemeli veya ilk deferred araç çağrısında tüm bilinen deferred araçları proaktif olarak yüklemelidir. Alternatif: deferred araç çağrı hatasını ajanın ToolSearch çağırması için uyarı veren özel bir hata tipi haline getirip ajanı durdurmayı (ask_user hatası gibi) önlemeli.

<!-- insight-sig:deferred_tool_called_without_schema -->

## Ertelenmiş MCP araçları şema yüklenmeden çağrılabiliyor

- **Severity:** med
- **Occurrences:** 1
- **Evidence sessions:** SES1307
- **File:** `internal/tools/ veya internal/api/ (araçların listelendiği ve sunulduğu yerler) ve MCP şema yükleme mantığı` ⚠️ (not found in repo — pointer unverified)

**Root cause:** TionHarness, create_task ve list_tasks gibi MCP araçlarını şema yüklenmeden aracı listesinde sunmakta, bu da ajanın bunları çağırabilmesine ve başarısız olmasına izin vermektedir. Hata mesajı sorunun ne olduğunu tanımlasa da, çözüme (ToolSearch çağrısı) doğru yönlendirmemekte ve tekrarlanan çağrılara karşı koruma bulunmamaktadır.

**Proposed fix:** Seçenek 1: Şeması yüklenmemiş araçları aracı listesinden hariç tutarak ToolSearch tarafından açıkça yüklendikten sonra kullanılabilir hale getirmek. Seçenek 2: Deferred araç çağrıldığında otomatik ToolSearch çağrısı yapıp şemayı yükleyerek işlemi yeniden denemek. Seçenek 3: Hata mesajını 'ToolSearch({"query": "select:create_task,list_tasks"}) ile şemayı yükleyin' şeklinde yönlendirici yaparak ajan rehberliği sağlamak.

<!-- insight-sig:deferred_mcp_tool_invocable_without_schema -->

## Task output tracking hardcodes Unix /tmp/ path, fails on Windows

- **Severity:** med
- **Occurrences:** 1
- **Evidence sessions:** SES1440
- **File:** `internal/tools/builtin_selfmanage.go` ⚠️ (not found in repo — pointer unverified)

**Root cause:** Task tracking sistemi geçici dosyaları `/tmp/tskN.out` olarak hardcode ediyor. Windows'ta Git Bash ortamında bu dosyalara erişilemiyor; sed komutları başarısız oluyor (TSK261, TSK265, TSK266, TSK267, TSK99999).

**Proposed fix:** Geçici dosya yolları dinamik olarak belirlenmelidir. Go kodunda `os.TempDir()` kullanılmalı veya shell betiklerinde platform-agnostic yöntem (`$(mktemp)` vb.) uygulanmalıdır.

<!-- insight-sig:task_tracking_unix_temp_windows_failure -->

## MCP araç adı öneki çiftleme hatası

- **Severity:** med
- **Occurrences:** 1
- **Evidence sessions:** SES1410
- **File:** `internal/providers/mcp.go ya da tool resolution middleware` ⚠️ (not found in repo — pointer unverified)

**Root cause:** MCP araç adını çözerken `mcp__tionharness_interaction__` ön eki iki kez uygulanıyor. Kullanıcı `mcp__tionharness_interaction__list_workers` istediğinde araç çözümleyicisi `mcp__tionharness_interaction__mcp__tionharness_interaction__list_workers` biçiminde arar ve bulamaz.

**Proposed fix:** MCP araç adı çözümleme kodunda ön ek yapıştırmasını kontrol et. Araç adının zaten MCP ön ekini içerip içermediğini kontrol ederek veya ön eki yalnız bir kez ekleyerek çiftlemeyi engelle.

<!-- insight-sig:mcp_tool_name_prefix_duplication -->

## System agent fallback model selection returns wrong model

- **Severity:** med
- **Occurrences:** 1
- **Evidence sessions:** SES1411
- **File:** `internal/agent/scheduler.go`

**Root cause:** Compactor system agent's embedded fallback model logic selects 'session-model' instead of the expected 'haiku' model when the system agent provider is incompatible

**Proposed fix:** Verify the fallback model resolution logic in the compactor system agent coordinator (internal/agent/scheduler.go or related system agent handler). Ensure that when a provider is incompatible, the fallback correctly routes to 'haiku' rather than 'session-model'.

<!-- insight-sig:systemagent_fallback_model_mismatch:compactor_haiku -->

## Lesson extractor system agent fallback prompt/model incorrect

- **Severity:** med
- **Occurrences:** 1
- **Evidence sessions:** SES1411
- **File:** `internal/agent/lessons_systemagent.go`

**Root cause:** The lesson extraction system agent's disabled-provider fallback is not returning the expected prompt or model type, likely due to incorrect fallback configuration in the system agent coordinator

**Proposed fix:** Review the fallback prompt and model selection in the lesson extractor system agent handler (internal/agent/ or internal/tools/). Ensure that when the production provider is unavailable, the fallback prompt and model are correctly configured and selected.

<!-- insight-sig:systemagent_fallback_logic_mismatch:lessonextractor_prompt_model -->

## MCP codebase-memory index operations fail with concurrency conflict

- **Severity:** med
- **Occurrences:** 1
- **Evidence sessions:** SES1411
- **File:** `internal/agent/agent.go` ⚠️ (not found in repo — pointer unverified)

**Root cause:** Agent dispatch may be spawning multiple concurrent `index_repository` calls for the same project without serialization, causing the MCP server to reject subsequent calls with 'another index operation for this project is active with different options'

**Proposed fix:** Add serialization or queueing logic in the MCP tool orchestration layer (likely in agent.go or tool call coordinator) to ensure index_repository operations for the same project are not executed concurrently. Either queue them or use a per-project lock.

<!-- insight-sig:mcp_codebase_memory_concurrent_index_conflict -->

## ask_user interactive olmayan oturum (autonomous run) içinde çağrılamıyor

- **Severity:** med
- **Occurrences:** 1
- **Evidence sessions:** SES1269
- **File:** `internal/providers/ or internal/agent/ (tool handler for ask_user in autonomous mode)` ⚠️ (not found in repo — pointer unverified)

**Root cause:** Ajan autonomous modda ask_user çağrısı yaptığında sistem 'no interactive session is available' hatası veriyor ve ajanı durduruyor. Bu, deferred araçların yüklenme hatası sırasında yaşananlar gibi, autonomous ajanların karar vermesi gereken noktalarda ajanı kilitliyor.

**Proposed fix:** ask_user autonomous modda çağrıldığında sistem "siz karar verin, devam edin" (proceed on your own) mesajını göstermek yerine, bu mesajı ajan prompt'una koymalı ve ajanın kendi başına karar vermesine izin vermelidir. Şimdiki davranış ajanı stopla, daha iyisi: ajanı kendiliğinden ileriye yönlendir.

<!-- insight-sig:ask_user_autonomous_no_fallback -->

## Proje zorunluluğuna karşın PowerShell'de bash sözdizimi komut başarısızlığı

- **Severity:** med
- **Occurrences:** 1
- **Evidence sessions:** SES1412
- **File:** `internal/providers/ (shell komut yönlendirme sistemi veya CLAUDE.md okuma mantığı)` ⚠️ (not found in repo — pointer unverified)

**Root cause:** Proje CLAUDE.md dosyasında 'Terminal: zorunlu Git Bash' belirtilmesine rağmen, TionHarness bash sözdizimi kullanan komutları (&&, ||, vs.) PowerShell 5.1 ortamında yürütmüştür. PowerShell 5.1 bu operatörleri deyim ayırıcısı olarak tanımadığından komut 'The token && is not a valid statement separator' hatası ile başarısız olmuştur.

**Proposed fix:** TionHarness, proje kökünde CLAUDE.md'deki 'Terminal: zorunlu Git Bash' gereksinimini okumalı ve shell komutlarını Git Bash ortamına yönlendirmelidir. Alternatif olarak, bash sözdizimi kullanan komutları otomatik olarak Bash tool'una yönlendirmeli veya PowerShell'de çalıştırılmaktan önce sözdizimini dönüştürmelidir.

<!-- insight-sig:shell_bash_syntax_in_powershell_5_1 -->

## Sistem ajanı kullanım çağrı türü yanlış biçimlendirildi

- **Severity:** med
- **Occurrences:** 1
- **Evidence sessions:** SES1382
- **File:** `internal/db/store_usage.go veya callkind_usage_test.go komşusu` ⚠️ (not found in repo — pointer unverified)

**Root cause:** usageCallKind() işlevi sistem ajanlarının anahtarlarını system:<provider>:<title> yerine sadece <title> veya system:fixed:<title> olarak oluşturuyor; sistem ajanı sağlayıcısı türü kayıt sırasında kaybolıyor veya yanlış sabitleniyor

**Proposed fix:** usageCallKind() işlevini inceleyip düzelt: sistem ajanı çağrılarında sağlayıcı ve başlık doğru şekilde ayrıştırılmalı, system:<provider>:<title> biçimi tutarlı şekilde üretilmeli; test başarıyla 'system:titler:title' formatını doğrulaması sağlanmalı

<!-- insight-sig:usageCallKind/system-agent-key-format-mismatch -->

## Üretilen kabuk betikleri geçersiz exit deyimi içeriyor

- **Severity:** med
- **Occurrences:** 1
- **Evidence sessions:** SES1382
- **File:** `internal/providers/claudecli.go veya internal/tools/builtin_spawn.go` ⚠️ (not found in repo — pointer unverified)

**Root cause:** Kabuk komutu oluşturma kodu, exit komutuna boş veya eksik sayısal argüman ile betikler oluşturuyor (örneğin 'exit' sonra hiçbir şey yerine 'exit 0'); bash bunu 'numeric argument required' hatasıyla reddediyor

**Proposed fix:** Exit komut oluşturma kodunu tüm kabuk betiği oluşturucularında doğrula: exit'in ardından her zaman sayısal bir çıkış kodu (0, 1 vb.) sağlanmalı; boş argüman üretimini önlemek için kabuk oluşturma koduna doğrulama ekle

<!-- insight-sig:shell-generation/exit-empty-argument -->

## Windows'ta geçici dizin temizliği unlinkat hatasıyla başarısız oluyor

- **Severity:** low
- **Occurrences:** 1
- **Evidence sessions:** SES1382
- **File:** `internal/api/spawn.go ve ilgili test dosyaları` ⚠️ (not found in repo — pointer unverified)

**Root cause:** TestSpawnWorkerInheritsCoordinatorCwd'de spawn/worker başlatma sırasında oluşturulan geçici dosyalar veya dizinler, test temizliği bunları silmeye çalışmadan önce düzgün şekilde kapatılmıyor veya serbest bırakılmıyor; Windows'ta dosyalar hala kullanımda olduğundan unlinkat başarısız oluyor

**Proposed fix:** Spawn/worker başlatma kodunda test temizliğinde tüm dosya tutamaçlarının düzgün şekilde kapatıldığından emin ol; test bırakma sırası geçici dizinlerde RemoveAll çağrılmadan önce tüm kaynakları kapatmalı; özellikle Windows'ta dosya kilidi sorunlarına karşı kontrol et

<!-- insight-sig:windows-cleanup/spawn-test-tempdir-locked -->

## MCP araç adı çiftleşmesi: tionharness_interaction araçları iki kez ön-ek alıyor

- **Severity:** high
- **Occurrences:** 1
- **Evidence sessions:** SES1410
- **File:** `internal/mcp/ veya internal/providers/ (MCP tool schema loading ve registration)` ⚠️ (not found in repo — pointer unverified)

**Root cause:** TionHarness, MCP sunucusundan yüklenen araç adlarını tool registration sırasında iki kez `mcp__<server>__` ön-eki ile işleyerek, `list_workers` gibi araçların `mcp__tionharness_interaction__mcp__tionharness_interaction__list_workers` şeklinde kayıt edilmesine neden oluyor. Araç lookup mekanizması çift ön-ekli adı bulamadığından 'No such tool available' hatası veriyor.

**Proposed fix:** MCP araç şeması yüklendiğinde, araç adının normalleştirmesini kontrol et: adda zaten `mcp__` ön-eki varsa tekrar ekleme. Veya araç adı kaydı (registration) sırasında ön-ek normalleştirmesinin idempotent olduğundan emin ol.

<!-- insight-sig:mcp_tool_name_double_prefix_tionharness_interaction -->

## Ertelenmiş araçlar otonom/subagent çalıştırmalarda şema yüklenmeksizin çağrılıyor

- **Severity:** med
- **Occurrences:** 1
- **Evidence sessions:** SES1269
- **File:** `internal/providers/claudecli.go veya araç dispatch kodu (araç çağrı interceptor'ı)` ⚠️ (not found in repo — pointer unverified)

**Root cause:** Otonom çalıştırmalarda veya subagent spawn'larında, TionHarness araç göndericisi deferred araçlar için şemaları otomatik olarak yüklemüyor. Ajanlar ToolSearch çağrısı yapmaksızın (ve çoğu zaman yapamaksızın) bu araçları çağırmaya kalkıyor, sonuçta 'deferred and cannot be called before its schema is available' hatası oluşuyor.

**Proposed fix:** Tool invocation layer'a, deferred araçlar için otomatik şema yükleme mantığı ekle: (1) Aracı çağırmadan önce şemanın yüklü olup olmadığını kontrol et; (2) Yüklü değilse ToolSearch ile yükle veya (3) Subagent sistem prompt'una otomatik şema yükleme talimatları dahil et, 'deferred tools require ToolSearch before use' mesajını içer.

<!-- insight-sig:deferred_tool_before_schema_loaded -->

## Deferred araçlar şema yüklenmeden çağrılabiliyor ve tekrarlanan başarısızlıklara yol açıyor

- **Severity:** low
- **Occurrences:** 1
- **Evidence sessions:** SES1307
- **File:** `internal/providers/claudecli.go`

**Root cause:** TionHarness, deferred araçları (create_task, list_tasks) mevcut araçlar listesinde sunuyor. Ajan veya kullanıcı bu araçları ToolSearch aracılığıyla şemalarını yüklemeden çağırabililiyor. Çağrı zamanında reddetme gerçekleştiğinden, tekrarlanan denemeler (4x create_task, 1x list_tasks) ve açık olmayan hata rehberliği görülüyor.

**Proposed fix:** ToolSearch çağrısı yapılıncaya kadar deferred araçları ajan tarafından callable olan araç listesinden hariç tut. Alternatif olarak, deferred araç çağrısı anında 'ToolSearch(select:<toolname>) ile şemayı yükle' şeklinde otomatik hata kılavuzu ver.

<!-- insight-sig:deferred_tool_schema_load_required -->

