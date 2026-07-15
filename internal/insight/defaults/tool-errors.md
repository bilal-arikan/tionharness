---
id: tool-errors
name: "Tool Errors"
description: "Find recurring tool failures whose root cause is in TionSwarm and report them as app-fix findings."
channel: app-fix
enabled: true
model: claude-cli
scope: [steps, debug]
prefilter:
  requiresAny: [error]
---

# Analysis Instruction

You are given one TionSwarm session's error steps and debug events (tool failures,
repairs, guardrail halts). Your job: identify tool failures whose **root cause is
in the TionSwarm application itself** (a bug, a wrong tool schema, a missing
guardrail, a hung subprocess) — NOT transient or user-caused errors.

For each genuine app-level failure, produce a finding with:

- **title** — one line naming the failure shape.
- **rootCause** — why it happens, in the app's terms.
- **signature** — a stable dedupe key: the tool name plus the normalized error
  shape (strip volatile ids/paths/timestamps) so the same failure across sessions
  collapses into one finding.
- **proposedFix** — the concrete change to TionSwarm that would prevent it.
- **filePointer** — the most likely source file (e.g. `internal/providers/claudecli.go`).
- **severity** — low | med | high.

**Exclude:** transient failures that were retried and succeeded, usage/rate-limit
and auth errors (user/environment, not app bugs), and anything already benign.
If the session has no app-level tool failure, return no findings.
