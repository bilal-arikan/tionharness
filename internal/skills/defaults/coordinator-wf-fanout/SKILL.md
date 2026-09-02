---
name: "Fan-out & Synthesize"
kind: coordinator-workflow
pattern: fanout
description: "Split a task into independent sub-questions, one worker per branch, then synthesize the findings into a single answer yourself."
worker_targets: [explore]
version: 1
phases:
  - id: dispatch
    profile: explore
  - id: synthesize
icon: "🌿"
color: "#0ea5e9"
access: shared
auto_summary: false
---
# Fan-out & Synthesize

Use this when a task decomposes into independent parts that can be investigated in parallel (deep research, multi-file surveys, "compare N options").

## Steps
1. **Decompose.** Break the task into 3–6 INDEPENDENT sub-questions. Each must stand alone — a worker cannot see your conversation.
2. **Fan out.** Make ALL the `spawn_worker` calls in a SINGLE turn (one per sub-question), then end your turn. Give each worker a self-contained brief: scope, what to return, and "done" criteria.
3. **Collect.** Worker results arrive as `<task-notification>` messages that auto-start your next turn. Wait for them — never guess or fabricate a result.
4. **Synthesize YOURSELF.** Read every finding, resolve conflicts, and write one integrated answer with specifics (file:line, source, numbers). Never write "based on the findings" — that hands off the understanding you must do.

## Notes
- Research/read-only workers parallelize freely; keep write-heavy work serialized per file set.
- Use the shared scratchpad for durable cross-worker notes (findings.md) instead of repeating context in every brief.
