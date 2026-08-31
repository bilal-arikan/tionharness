---
name: "Loop Until Done"
kind: coordinator-workflow
pattern: loop
description: "Keep spawning workers round after round until a stop condition holds (no new findings for N rounds). For open-ended discovery."
worker_targets: [explore]
stop_condition: "two consecutive rounds produce no new findings"
max_turns: 10
icon: "🔁"
color: "#8b5cf6"
access: shared
auto_summary: false
---
# Loop Until Done

Use this for unbounded discovery where you don't know how much work remains: exhaustive bug hunts, "find everything of type X", iterative refinement.

## Steps
1. **Round.** Spawn one or more workers for the current round with a precise brief.
2. **Assess.** When their `<task-notification>` results arrive, compare against everything found so far (keep a running list in the shared scratchpad, e.g. findings.md). Extract only what is NEW.
3. **Decide.** If the round produced new results, start another round. If the STOP CONDITION holds (see below), stop.
4. **Synthesize & report** the accumulated result once you stop.

## Stop condition
Stop when two consecutive rounds add nothing new (dedupe against the running list, not against a filtered subset — or rejected items reappear forever). The notify-loop is also hard-capped by max_turns as a safety backstop; if you hit it, report what you have and tell the user.

## Notes
- Track "seen" items durably in the scratchpad so dedup survives across rounds and workers.
