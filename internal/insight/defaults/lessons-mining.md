---
id: lessons-mining
name: "Lessons Mining"
description: "Distil durable, reusable lessons from failed sessions to feed the lessons store."
channel: workspace-opt
enabled: true
model: claude-cli
scope: [steps, debug]
prefilter:
  requiresAny: [error]
---

# Analysis Instruction

You are given one session that hit errors. Extract **durable lessons** — general rules that would
help a future agent AVOID the same failure. A good lesson is transferable, not a one-off fact.

Look for:

- A mistake the agent made and only later corrected → the corrective rule is the lesson.
- A tool/API misused in a way that predictably fails (wrong arg shape, missing step, ordering) →
  the correct usage is the lesson.
- An environment/platform gotcha (path form, shell quirk, permission) the agent tripped on.

For each, produce a workspace-opt finding whose fields double as a lesson:

- **title** — the lesson as a short imperative ("Quote Windows paths containing spaces").
- **rootCause** — the failure it prevents.
- **proposedFix** — the rule to follow next time (this becomes the lesson body).
- **severity** (low|med|high) — how costly repeating the mistake is.
- **signature** — stable dedupe key for the lesson (e.g. `lesson:windows-path-quoting`).

The signature is the identity of the lesson across runs, not a label for this run. When the
signatures already on record are listed below and one of them names the same problem, reuse it
CHARACTER FOR CHARACTER — do not reword it, reorder it, or swap a synonym in. A new slug for a
known problem stores the same lesson twice and both copies then look like one-offs.

Name the problem, not the occurrence: `lesson:crlf-shebang` covers every CRLF-shebang failure,
while `lesson:crlf-shebang-breaks-deploy-script` invites a new slug for the next script.

Findings from this lens are additionally promoted into the runtime **lessons store** so future
turns carry them. Only emit lessons that are genuinely reusable; skip session-specific noise.
