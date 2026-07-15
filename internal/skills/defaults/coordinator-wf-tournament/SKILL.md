---
name: "Tournament"
kind: coordinator-workflow
pattern: tournament
description: "Compare candidates in pairwise judged rounds, eliminating losers until one winner remains. For picking the single best solution."
worker_targets: [reviewer]
icon: "🏆"
color: "#eab308"
access: shared
auto_summary: false
---
# Tournament

Use this when you must pick the ONE best option from several strong candidates and a flat rubric score isn't decisive — pairwise comparison surfaces the winner.

## Steps
1. **Seed the bracket.** List the candidates (from a prior Generate & Filter round, or given). State the head-to-head criterion.
2. **Judge pairwise.** For each pair, spawn a fresh `spawn_worker` judge with both candidates and the criterion; ask for a winner + a one-line reason. Run independent pairs concurrently (one turn, many spawns) — their notifications coalesce into your next turn.
3. **Advance winners.** Collect verdicts, drop losers, and run the next round on the survivors. Repeat until one remains.
4. **Report the winner** and the deciding comparisons; keep runners-up notes for grafting good ideas.

## Notes
- Fresh judges per pair avoid anchoring bias; never let a candidate judge itself.
- For an odd count, give one candidate a bye that round.
