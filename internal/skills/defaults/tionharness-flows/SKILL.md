---
name: "TionHarness Flows"
description: "Detailed guide to building and running orchestration flows: multi-step, multi-agent graphs with inputs, dependencies and a recorded run history."
when_to_use: "When designing, creating or running a flow (multi-step or multi-agent orchestration) in TionHarness"
icon: "🔀"
color: "#0ea5e9"
access: shared
auto_summary: false
---
# TionHarness Flows — Detailed Guide

A **flow** is a directed graph of steps that orchestrates one or more agents to
complete a larger task. Each run is recorded as a session in the executions feed,
so a flow run is replayable like any chat.

## Anatomy of a flow

- **Steps (nodes)** — each step delegates a prompt to a chosen agent. A step may
  consume the output of earlier steps it depends on.
- **Edges (dependencies)** — declare which steps must finish before a step runs.
  TionHarness deep-validates the graph before running. **Cycles are allowed** — a
  `branch` can route back to an earlier node to form an iteration loop; the engine
  bounds any loop with a step cap (`maxSteps`, 50). See `[[tionharness-gan-loop]]` for
  the generator↔evaluator refine/pivot loop built on a back edge.
- **Input** — the flow's initial input is threaded to entry steps.

## Graph JSON schema (node types & fields)

The `graph` argument is a JSON **string** of `{"start":"<id>","nodes":[...]}`.
Each node's fields depend on its `type` — use the exact field names below
(common mistake: using `branches`/`next` on a parallel node):

| Type | Required fields | Continues via |
|------|-----------------|---------------|
| `start` | (none) — REQUIRED, exactly one; the graph `start` must be its id | `next` (the first real node) |
| `end` | (optional) `template` (shape output), `outputSchema` (JSON Schema the final output must satisfy, else the run fails) | terminal — no `next` |
| `spawn` | `spawnFlows` (child flow ids launched ASYNC/non-blocking), `template` (input) | `next` |
| `join` | (optional) `spawnRef` (which spawn node to await; "" = all), `joinTimeoutSec` (0 = forever), `joinPartial` (drop failed/suspended/timed-out children instead of failing) — barrier that block-waits the spawned runs, joins outputs into `{{last}}` | `next` |
| `agent` | `agentId` (an existing agent's exact ID, such as `AGT6`; never a node id or agent name), `prompt` | `next` (node id; `""` = end) |
| `parallel` | `parallel`: **array of child agent node ids** | `joinNext` (node after the join) |
| `branch` | `branches`: array of `{contains, next}` rules | per-arm `next` |
| `delay` | `delayMs` | `next` |
| `transform` | `template` | `next` |
| `loop` | `body` (loop entry id), and `maxIters`>0 or non-empty `until` | `loopNext` (node after exit) |
| `await-input` | (optional `timeoutSec`) | `next` — the run PAUSES until input arrives (durable), then continues with it as `{{last}}` |
| `subflow` | `flowRef` (child flow id), optional `template` (child input; default `{{last}}`) | `next` — runs the child flow to completion, captures its output; if the child suspends at `await-input` the PARENT run also suspends and feeding it resumes the child (propagation) |

Parallel fan-out + join example (run `a` and `b` concurrently, then `merge`):

```json
{"start":"fan","nodes":[
  {"id":"fan","type":"parallel","parallel":["a","b"],"joinNext":"merge"},
  {"id":"a","type":"agent","agentId":"AGT6","prompt":"Angle A: {{input}}"},
  {"id":"b","type":"agent","agentId":"AGT7","prompt":"Angle B: {{input}}"},
  {"id":"merge","type":"agent","agentId":"AGT8","prompt":"Merge {{node.a}} and {{node.b}}","next":""}
]}
```

Parallel children must be `agent` nodes. Templates: `{{input}}`, `{{last}}`,
`{{node.<id>}}` (plus `{{iteration}}`, the 0-based loop counter, inside a `loop` body).

A `loop` repeats its `body` sub-chain (which must terminate with `next:""`) until
`maxIters` or an `until` match on the last output, then continues at `loopNext`.
Set the graph-level `"accumulate": true` to make sequential agent nodes share one
growing conversation thread so the prompt cache is reused across nodes (a node can
opt out with `"fresh": true`); off by default = each node is a stateless call.

## Building a flow (self-management tools)

These tools require self-management to be enabled for the workspace:

- `create_flow` — define a flow (name, steps, dependencies).
- `update_flow` — edit an existing flow.
- `get_flow` / `list_flows` — inspect definitions.
- `delete_flow` — remove an agent-created flow.
- `run_flow` — drive a flow to completion (autonomous, budget-gated); the run is
  recorded in the executions feed.
- `list_flow_runs` — list flow runs; use `status="waiting"` to find runs paused at an
  `await-input` node (each row's `waitingAt` is the await node id).
- `deliver_flow_input` — feed input to a WAITING run (from `list_flow_runs`), resuming it;
  the input becomes `{{last}}` for the node after the await. Lets an agent/coordinator drive a
  waiting flow, not just a human in the UI.

## Design checklist

1. **Decompose** the goal into discrete steps, each with a single clear intent.
2. **Assign** each step to the agent best suited for it.
3. **Wire dependencies** so a step only starts once its inputs exist. Cycles are
   allowed for iteration loops (a `branch` routing back to an earlier node); the
   engine caps total steps so a runaway loop always terminates.
4. **Validate** before running — every `agentId` must be an existing agent's exact
   ID, not its display name or the node's `id`; fix any cycle or dangling-dependency
   error the graph validator reports.
5. **Run** via `run_flow`, then review the recorded run session.

## Tips

- Prefer several small, single-purpose steps over one broad step — they are
  easier to validate, retry and reason about.
- A step's prompt should reference the outputs it depends on explicitly.
- Budget guards apply to autonomous runs; keep step count proportional to the
  task.
