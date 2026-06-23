---
name: "SwarmGo Flows"
description: "Detailed guide to building and running orchestration flows: multi-step, multi-agent graphs with inputs, dependencies and a recorded run history."
when_to_use: "When designing, creating or running a flow (multi-step or multi-agent orchestration) in SwarmGo"
icon: "🔀"
color: "#0ea5e9"
access: shared
auto_summary: false
---
# SwarmGo Flows — Detailed Guide

A **flow** is a directed graph of steps that orchestrates one or more agents to
complete a larger task. Each run is recorded as a session in the executions feed,
so a flow run is replayable like any chat.

## Anatomy of a flow

- **Steps (nodes)** — each step delegates a prompt to a chosen agent. A step may
  consume the output of earlier steps it depends on.
- **Edges (dependencies)** — declare which steps must finish before a step runs.
  The graph must be acyclic; SwarmGo deep-validates it before running.
- **Input** — the flow's initial input is threaded to entry steps.

## Graph JSON schema (node types & fields)

The `graph` argument is a JSON **string** of `{"start":"<id>","nodes":[...]}`.
Each node's fields depend on its `type` — use the exact field names below
(common mistake: using `branches`/`next` on a parallel node):

| Type | Required fields | Continues via |
|------|-----------------|---------------|
| `agent` | `agentId`, `prompt` | `next` (node id; `""` = end) |
| `parallel` | `parallel`: **array of child agent node ids** | `joinNext` (node after the join) |
| `branch` | `branches`: array of `{contains, next}` rules | per-arm `next` |
| `delay` | `delayMs` | `next` |
| `transform` | `template` | `next` |

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
`{{node.<id>}}`.

## Building a flow (self-management tools)

These tools require self-management to be enabled for the workspace:

- `create_flow` — define a flow (name, steps, dependencies).
- `update_flow` — edit an existing flow.
- `get_flow` / `list_flows` — inspect definitions.
- `delete_flow` — remove an agent-created flow.
- `run_flow` — drive a flow to completion (autonomous, budget-gated); the run is
  recorded in the executions feed.

## Design checklist

1. **Decompose** the goal into discrete steps, each with a single clear intent.
2. **Assign** each step to the agent best suited for it.
3. **Wire dependencies** so a step only starts once its inputs exist. Keep the
   graph acyclic.
4. **Validate** before running — fix any cycle or dangling-dependency error the
   graph validator reports.
5. **Run** via `run_flow`, then review the recorded run session.

## Tips

- Prefer several small, single-purpose steps over one broad step — they are
  easier to validate, retry and reason about.
- A step's prompt should reference the outputs it depends on explicitly.
- Budget guards apply to autonomous runs; keep step count proportional to the
  task.
