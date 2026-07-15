# TionSwarm — Insight App-Fix Backlog

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

**Root cause:** The agent attempted to call `get_flow` both as a bare tool name and as `mcp__tionswarm_extended__get_flow`, but neither is registered in the tool registry — indicates either a missing tool registration/export in the MCP extended toolset or a stale tool name surfaced to the model (e.g. in a prompt/schema hint) that doesn't match what's actually wired up in internal/tools/registry.go.

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

