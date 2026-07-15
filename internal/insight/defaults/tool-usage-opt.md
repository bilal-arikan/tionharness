---
id: tool-usage-opt
name: "Tool Usage Optimization"
description: "Spot inefficient tool usage patterns and propose workspace tool-config changes."
channel: workspace-opt
enabled: true
model: claude-cli
scope: [steps, toolCalls]
prefilter:
  minCount: { tool: 12 }
---

# Analysis Instruction

You are given one tool-heavy session's steps. Identify inefficient TOOL usage the workspace
could fix through configuration (not app code). Look for:

- The same tool called **repeatedly with tiny variations** where one call (or a different tool)
  would do — wasted turns/tokens.
- A tool that is **never useful for this agent** yet keeps getting offered/tried → propose adding
  it to the agent's blocked tools or the workspace disabled tools.
- **Deferred/lazy tools repeatedly re-searched** (`tool_search`/`activate_tools` churn) → propose
  making the needed tool eager or trimming the catalog.
- A cheaper native tool ignored in favor of an expensive one.

For each, produce a workspace-opt finding:

- **title**, **rootCause**, **severity** (low|med|high).
- **signature** — stable dedupe key: tool name + issue kind (e.g. `Grep:repeated-narrowing`).
- **proposedFix** — the concrete workspace change (block/disable a tool, make one eager, adjust
  the agent's tool set), actionable by the user.

If tool usage was efficient, return no findings.
