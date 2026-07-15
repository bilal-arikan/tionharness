---
id: skill-usage-opt
name: "Skill Usage Optimization"
description: "Analyze how skills were used and propose workspace changes to make them cheaper and more effective."
channel: workspace-opt
enabled: true
model: claude-cli
scope: [steps, toolCalls]
prefilter:
  requiresAny: [use_skill]
---

# Analysis Instruction

You are given one session's steps, focusing on skill usage (use_skill invocations and
what followed). Identify how the workspace's skills could be optimized. Look for:

- A skill **referenced/loaded but not actually used** (loaded, then ignored) → context bloat.
- A skill **read but ineffective** (the agent still failed or ignored its guidance).
- A **bloated skill** whose body is far larger than what the task needed.
- A **missing skill** — a repeated task pattern that a small skill would have made reliable.

For each, produce a workspace-opt finding:

- **title** — the optimization in one line.
- **rootCause** — what about the current skill setup caused the inefficiency.
- **signature** — a stable dedupe key: the skill slug + issue kind (e.g. `tionswarm-flows:loaded-unused`).
- **proposedFix** — the concrete workspace change (trim skill body, mark name-only, remove from
  agent, add a new skill) — actionable by the user, not code in the app.
- **severity** — low | med | high.

If skill usage was efficient, return no findings.
