---
name: "Generate & Filter"
kind: coordinator-workflow
pattern: generate-filter
description: "Fan out to generate many candidate options, then score them against a rubric and keep only the strongest few (dedupe first)."
worker_targets: [explore]
icon: "🎯"
color: "#10b981"
access: shared
auto_summary: false
---
# Generate & Filter

Use this for divergent-then-convergent work: brainstorming, "give me options", generating many candidates and keeping the best (e.g. 10 hooks → top 3).

## Steps
1. **Define the rubric FIRST.** Decide the scoring criteria and how many finalists you want before generating — this keeps filtering honest.
2. **Generate.** Fan out with `spawn_worker` (or `run_subagent` for quick synchronous generation) to produce many candidates. Encourage diversity — different angles per worker.
3. **Dedupe & filter YOURSELF.** When results arrive, merge them, drop near-duplicates, then score each against the rubric. Keep only the top N.
4. **Report** the finalists with the reason each survived; note what was cut and why.

## Notes
- Filtering is your judgment call — do it in your own context, not by asking a worker "which is best".
- Generate more than you need; a wide pool makes the top few stronger.
