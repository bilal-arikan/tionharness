---
id: workspace-tuning
name: "Workspace Asset Tuning"
description: "Turn recurring session friction into concrete changes to workspace ASSETS — skills, agents, tool config, hooks/automations — before ever touching CLAUDE.md."
channel: workspace-opt
enabled: true
model: claude-cli
scope: [steps, debug, toolCalls]
prefilter:
  requiresAny: [error, recovery, guardrail, lesson]
---

# Analysis Instruction

You are given one session's error/recovery steps and debug events (errors, repairs, guardrail
decisions, distilled lessons). Diagnose friction that the WORKSPACE could have prevented, and
propose the fix as a change to a workspace ASSET.

## Where the fix must land (strict priority order)

Walk this list top-down and stop at the FIRST target that can actually carry the fix. Only
propose a target lower on the list after you have ruled out every target above it.

1. **Skill** — a missing or inadequate skill. The agent had to rediscover a procedure, guessed a
   command/path wrong, or repeated a multi-step ritual that no skill described. Propose writing a
   new skill (give its slug and the sections it needs) or amending a named existing one (give the
   slug and the exact rule to add or correct).
2. **Agent configuration** — the wrong agent shape caused it: the soul/system prompt lacks a
   standing rule, the model tier is mismatched to the work, the allowed/blocked tool set is wrong,
   or a subagent profile is missing or misused. Name the agent and the field to change.
3. **tools-config** — a tool that is never useful here keeps being offered or keeps failing;
   propose disabling it, or moving it to deferred/lazy so it costs nothing until searched. Name
   the tool exactly as it appears in the steps.
4. **Hook or automation** — a manual step was repeated on a schedule or after a recurring event
   (format after edit, re-index after a change, notify on a state transition). Propose the hook
   (which event, which command) or the automation (which trigger, which target).
5. **Insight settings** — the friction is in the scan itself: a lens firing on sessions holding no
   evidence for it, a prefilter too broad or too narrow, maxSessions/maxAnalyzed set wrong, or a
   lens that should be disabled. Name the lens and the setting.
6. **CLAUDE.md note** — LAST RESORT ONLY. Allowed only when the fix is a plain, always-on project
   fact that none of the five targets above can hold. If a skill, agent field, tool toggle, hook or
   automation could carry it, that is the finding — not a CLAUDE.md line.

## Finding contract

For each, produce a workspace-opt finding:

- **title** — the asset change in one line.
- **rootCause** — which missing/misconfigured workspace asset produced the observed friction.
- **signature** — a stable dedupe key: target asset + issue kind, e.g.
  `skill:tionharness-build:missing-test-cmd`, `agent:Builder:model-too-small`,
  `tools-config:WebFetch:always-fails`.
- **proposedFix** — concrete and applicable: the exact skill slug and the rule to add, the exact
  agent field and its new value, the exact tool name to disable, the exact hook event and command.
  Never "improve the docs" or "add guidance" — say which asset, which field, which text.
- **filePointer** — the target asset, in the same form as the signature's head:
  `skill:<slug>`, `agent:<name>`, `tools-config.json`, `hook:<event>`, `automation:<name>`,
  `insight:lens:<id>`, or `CLAUDE.md` for the last-resort case.
- **severity** — low | med | high.

If the friction was transient (provider/auth/network) or is a bug in TionHarness itself rather
than a workspace configuration gap, return no findings.
