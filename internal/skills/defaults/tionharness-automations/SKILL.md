---
name: "TionHarness Automations"
description: "The Automation entity in TionHarness: event-driven rules that fire a target agent when something happens — a tagged session finishing, a kanban card changing, cumulative token spend crossing a threshold, or a session's message/tool count crossing an interval. Covers every trigger kind, the full config surface (session mode, guardrails, targeting, prompt placeholders), the self-loop model, and ready recipes. Distinct from cron Schedules and Hooks."
when_to_use: "When you (or a user) want an agent to run automatically in reaction to an EVENT rather than a clock — e.g. 'when a card enters Review run the reviewer', 'every 150k tokens run a maintenance pass', 'every 20 messages summarize progress', 'when a session gets tagged stuck, spawn a fixer'. Also when choosing between spawning a fresh session per fire vs continuing one persistent thread, or when a create_automation / update_automation call is being written."
icon: "⚡"
color: "#f59e0b"
access: shared
---
# TionHarness — Automations

An **Automation** is an event-driven rule: *when X happens, render a prompt and run
a target agent* (whose own evolving flow shapes the turn). It is a distinct entity from cron **Schedules** (clock
triggers, `tionharness-autonomous-ops` §4) and **Hooks** (tool-call interception) —
reach for an Automation when the trigger is a domain EVENT, not a timer.

> Exact field names/enums: the **`create_automation` / `update_automation` tool
> schemas are authoritative** — this skill is the concept + recipe layer. Deep
> mechanics (engine, fire path, self-trigger guards): repo `_Docs/46-ETIKET-OTOMASYON.md`.
> The tools live in `tionharness-self-management`; targets come from `list_agents`.

## Tools

`create_automation` · `update_automation` (partial patch — send only changed fields)
· `list_automations` · `delete_automation` · toggle (enable/disable) · reset (clear
the iteration counter). All surface in the **Otomasyon** (Schedules) screen too.

## Trigger kinds (`triggerKind`)

| Kind | Fires when… | Key fields | Prompt vars |
|------|-------------|-----------|-------------|
| `tag` (default) | a session carrying `triggerTag` finishes a turn | `triggerTag`, `spawnTags` | `{{result}}` `{{title}}` `{{tag}}` `{{sessionId}}` `{{prevPrompt}}` `{{agent}}` |
| `board` | a kanban card changes (move/create/update/delete) | `boardOp`, `boardFromState`/`boardToState`, `boardAction`, `boardPriority`, `boardExclusive` | `{{taskId}}` `{{title}}` `{{op}}` `{{from}}`/`{{to}}` `{{toLabel}}` `{{owner}}` `{{priority}}` `{{tags}}` |
| `token` | cumulative token spend crosses each `tokenThreshold` multiple | `tokenScope` (session\|workspace), `tokenThreshold` (min 1000) | `{{tokens}}` `{{threshold}}` `{{scope}}` `{{sessionId}}` |
| `counter` | a session's message/tool count crosses each `counterInterval` multiple | `counterMetric` (message\|tool), `counterScope` (session\|workspace), `counterInterval` (min 2) | `{{count}}` `{{interval}}` `{{metric}}` `{{sessionId}}` |
| `flow` | an agent's flow run (one turn of its evolving flow, `_Docs/93`) finishes | `flowAgentId` ("" = any agent), `flowStatus` (""\|success\|failure), `flowMaxGrade` (0 = ignore; 1..5 = only runs the decision model graded at or below it — needs the `flow-grade` authority on) | `{{result}}` (reply or error) `{{input}}` `{{output}}` `{{error}}` `{{status}}` `{{grade}}` `{{runId}}` `{{flowId}}` `{{flowName}}` `{{version}}` `{{agent}}` `{{agentId}}` `{{sessionId}}` |

Common vars in every kind: `{{iteration}}` `{{maxIterations}}` `{{automation}}`
`{{date}}` `{{time}}` `{{datetime}}`. **Prefer `counter` over `token`** for a stable
per-conversation cadence — token counts are cache-inflated and fire unpredictably.

## Target

- `targetAgentId` — spawn/continue a session on this agent.
- Exception: a `board` rule with `boardAction:"archive"` archives the card with **no
  LLM call** and needs no target.

## Session mode (`sessionMode`, agent-backed only)

- `spawn` — a **fresh session** every fire (default for tag/board). Fires are
  independent.
- `continue` — one **persistent per-automation thread** that carries prior turns
  forward, **history-aware** (default for token/counter — the cron-schedule feel).
  Continue does not seed the tag self-loop and ignores `spawnTags`/parent-tag
  clearing.
- Omit to get the per-kind default. "Read prior conversation" is simply `continue`.

## Guardrails (bound the loop — always set a stopping condition)

- `maxIterations` — total fires before auto-disable. **Range 1–500; 0/unlimited is
  rejected.** Default 50. (Legacy `<=0` rows are backstopped at 1000.)
- `cooldownSec` — minimum seconds between fires.
- `expiresAt` — optional unix-seconds end date (0 = none); past it, auto-disables.
- `enabled` — kill switch; **enabling resets the iteration counter**.
- Everything is also bounded by the per-workspace **autonomy brake** (`pauseAutonomy`).

## The self-loop (tag kind)

A `tag` rule's spawned session is tagged with `spawnTags`, which **defaults to
`[triggerTag]`** — so the new session re-fires the same rule on completion, forming
a bounded loop (`maxIterations`). Pass `spawnTags: []` to **break** the loop (fire
once, no chain). Board/token/counter/flow rules never self-loop; a `flow` rule also
skips the run of the very session it spawned.

## Flows can fire automations too

A flow's `trigger` node (see the `tionharness-flows` skill) fires ANY automation —
whatever its kind — from inside a run, with the node's rendered payload as
`{{result}}` plus the flow vars above. The automation's own guards (enabled,
cooldown, iteration cap, autonomy brake) still apply; a declined fire is noted on
the run step and never fails the agent's turn. Use a `flow`-kind rule for
"after the run" reactions (review poor replies, log failures) and a `trigger`
node for "during the run" hand-offs.

## Recipes

- **Docs sync every 150k tokens:** `triggerKind:"token"`, `tokenScope:"workspace"`,
  `tokenThreshold:150000`, `sessionMode:"continue"`, target = a maintenance agent,
  prompt using `{{tokens}}`/`{{threshold}}`. Set `maxIterations` to a real cap (e.g. 200).
- **Board drives execution:** `triggerKind:"board"`, `boardToState:"in_progress"`,
  `boardAction:"spawn"`, target = worker → moving a card starts an agent on it. Pair
  with a `boardToState:"done"`, `boardAction:"archive"` rule for cleanup.
  **Note:** every workspace is seeded with exactly this pair of rules
  (`board-run-in-progress` / `board-archive-done`) but **`Enabled:false` by
  default** (`internal/agent/automation_defaults.go`) — board-driven execution is
  opt-in, the user (or you) must explicitly enable them before moving a card does
  anything. The bundled blank workspace also seeds two separate **enabled** PM
  rules for cards entering `failed` or `review`; both use `sessionMode:"continue"`.
- **Per-conversation checkpoint:** `triggerKind:"counter"`, `counterMetric:"message"`,
  `counterInterval:20`, `sessionMode:"continue"` → every 20 messages the agent
  summarizes progress in the same thread.
- **Stuck-session fixer:** `triggerKind:"tag"`, `triggerTag:"stuck"`, `spawnTags:[]`
  (fixer must not carry `stuck`), target = a debugger agent. On success the framework
  clears the parent's `stuck` tag automatically (`_Docs/46` §3).

## Pitfalls

- **No stopping condition** = runaway until `maxIterations`/brake. Always cap it.
- **`continue` breaks the tag self-loop** — the maintenance thread carries no trigger
  tag, so it will not re-fire itself. Use `spawn` when you want the tag loop.
- **Switching kind via update** must leave a valid shape (a `tag` rule needs a
  `triggerTag`; a spawn rule needs a target) — the server rejects incoherent edits
  (`ValidateAutomationShape`).
- **Archived rules and targets** — an archived automation never fires (fire ledger
  reason `archived`); a rule whose target agent is archived is skipped with reason
  `agent_archived` instead of spawning a turn that would fail. `list_automations`
  hides archived rules unless called with `archived:true`. Restore with
  `POST /api/automations/{id}/unarchive` (or unarchive the agent) — do not recreate.
- **Token vs counter cadence** — token thresholds fire unpredictably (cache-inflated);
  reach for `counter` when you want a steady rhythm.
