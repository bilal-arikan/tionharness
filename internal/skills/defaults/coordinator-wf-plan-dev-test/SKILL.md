---
name: "Plan → Development → Test"
kind: coordinator-workflow
pattern: custom
description: "Deliver one feature/fix as three separate workers — a planner, an implementer, and an independent validator — with a bounded repair loop, then report a single compact result upward."
worker_targets: [planner, coder, validator]
stop_condition: "the validator returns VERDICT: PASS, or the repair budget is spent"
max_turns: 24
icon: "🧩"
color: "#10b981"
access: shared
auto_summary: false
---
# Plan → Development → Test

Use this when you have been handed ONE unit of work (a feature, a fix, a card) and you own it end to end. Three different workers do the three jobs, so no single context holds the research, the diff and the logs at once — and the verdict comes from eyes that did not write the code.

You are the only one who sees all three. Nobody else in this tree should ever need to read a diff.

## Steps

1. **Plan.** `spawn_worker` a **`planner`** with a self-contained brief: the goal, the repo/paths you already know, and what "done" means. It returns GOAL / FILES / STEPS / VERIFY / RISKS. Do not plan the change yourself — but DO read the plan critically when it lands. If it names the wrong files or misses the goal, send it back once with the correction rather than passing a bad plan downstream.

2. **Develop.** `spawn_worker` a **`coder`** with the planner's output **pasted in full** as its brief (it cannot see the planner's session). Tell it explicitly: implement the plan, run the VERIFY command itself, and report files-changed plus a one-line rationale each. Do NOT let it commit yet.

3. **Test.** `spawn_worker` a **`validator`** — always fresh, never the implementer — with the goal and the VERIFY command. It runs tests/typecheck/build without editing source and returns a compact `VERDICT: PASS | FAIL`.

4. **Repair (bounded).** On `FAIL`, `send_to_worker` the ORIGINAL implementer with just the validator's failing lines — it still has the files loaded — then spawn a fresh validator again. Allow **at most 2 repair rounds**. If it still fails, stop and report `failed` upward with the validator's notes; do not keep grinding.

5. **Commit & report.** Only after a `PASS`, tell the implementer to commit its own work. Then synthesize ONE compact result: what changed (files), the verdict line, and anything the caller must know. If you are a sub-coordinator, deliver it with `report_to_coordinator`.

## Rules

- **Never do the three jobs yourself.** Reading the plan and the verdict is your job; reading the diff is not. If you find yourself opening source files, you have taken over the implementer's work and your context is now the bottleneck for everything above you.
- **Each brief must stand alone.** Workers cannot see your conversation or each other's sessions. Paste the plan; don't reference it.
- **The validator never edits.** A verdict on code the verifier quietly fixed proves nothing.
- **A FAIL is unfinished work.** Never commit, conclude, or report success on one.
- Use the shared scratchpad (plan.md) for the plan instead of re-pasting it across several repair rounds.
