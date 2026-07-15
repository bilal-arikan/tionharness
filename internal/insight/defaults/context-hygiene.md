---
id: context-hygiene
name: "Context Hygiene"
description: "Detect unnecessary or missing context (CLAUDE.md, config prompts, agent soul) that caused friction and propose workspace fixes."
channel: workspace-opt
enabled: true
model: claude-cli
scope: [steps, debug, context]
prefilter:
  requiresAny: [error, cache_break]
---

# Analysis Instruction

You are given one session's error/recovery steps and debug events (including cache breaks).
Diagnose CONTEXT problems — what the agent was or wasn't told — that caused the friction. Look
for two opposite failure modes:

- **Missing context** — the agent lacked a fact it needed (project convention, a path, a
  constraint) and failed or thrashed because of it → propose adding it to the right place
  (workspace CLAUDE.md, the agent's soul/system prompt, a config prompt).
- **Unnecessary / stale context** — the agent was fed bloated or wrong context that misled it or
  wasted tokens / broke the prompt cache → propose trimming or correcting it.

For each, produce a workspace-opt finding:

- **title** — the context fix in one line.
- **rootCause** — which missing/excess context caused the observed friction.
- **signature** — a stable dedupe key: context target + issue kind (e.g. `claude-md:missing-build-cmd`).
- **proposedFix** — the concrete change to workspace context (add/trim a specific line), actionable
  by the user.
- **severity** — low | med | high.

If the friction was not context-related (transient/provider/auth), return no findings.
