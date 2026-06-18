---
name: "SwarmGo Flows"
description: "Detailed guide to building and running orchestration flows: multi-step, multi-agent graphs with inputs, dependencies and a recorded run history."
when_to_use: "When designing, creating or running a flow (multi-step or multi-agent orchestration) in SwarmGo"
icon: "🔀"
color: "#0ea5e9"
access: shared
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
