---
id: context-hygiene
name: "Context Hygiene"
description: "Detect missing or stale PROMPT TEXT that caused friction. Asset-shaped fixes (skill/agent/tool/hook) belong to the workspace-tuning lens."
channel: workspace-opt
enabled: true
model: claude-cli
scope: [steps, debug, context]
prefilter:
  requiresAny: [error, cache_break]
---

# Analysis Instruction

You are given one session's error/recovery steps and debug events (including cache breaks).
Diagnose CONTEXT problems ONLY — what the agent was or wasn't told, as text in its prompt. Look
for two opposite failure modes:

- **Missing context** — the agent lacked a plain project FACT it needed (a convention, a path, a
  constraint) and failed or thrashed because of it → propose adding it where that fact belongs.
- **Unnecessary / stale context** — the agent was fed bloated or wrong context that misled it or
  wasted tokens / broke the prompt cache → propose trimming or correcting it.

**Scope guard — do not produce a finding when a workspace ASSET is the real fix.** If the friction
would be prevented by writing or amending a skill, changing an agent's soul / model / tool set,
disabling or deferring a tool, or adding a hook or automation, that is NOT a context-hygiene
finding: stay silent and leave it to the `workspace-tuning` lens. Only report here when the fix is
genuinely a piece of prompt text — a fact to add, trim or correct. In particular, do not reach for
"add a note to CLAUDE.md" as the default remedy; it is correct only for an always-on project fact
that no workspace asset can hold.

For each remaining context problem, produce a workspace-opt finding:

- **title** — the context fix in one line.
- **rootCause** — which missing/excess context caused the observed friction.
- **signature** — a stable dedupe key: context target + issue kind (e.g. `claude-md:missing-build-cmd`).
- **proposedFix** — the concrete change to workspace context (add/trim a specific line), actionable
  by the user.
- **severity** — low | med | high.

If the friction was not context-related (transient/provider/auth), return no findings.
