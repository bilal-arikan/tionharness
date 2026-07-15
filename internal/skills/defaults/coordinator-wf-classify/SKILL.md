---
name: "Classify & Act"
kind: coordinator-workflow
pattern: classify
description: "Classify the incoming task by type, then route it to the right worker profile (explore / coder / reviewer) for that category."
worker_targets: [explore, coder, reviewer]
icon: "🗂️"
color: "#f59e0b"
access: shared
auto_summary: false
---
# Classify & Act

Use this when incoming requests are heterogeneous and each type wants a different handler: mixed support tickets, "triage this backlog", routing work by kind.

## Steps
1. **Classify.** Read the task and decide its category (e.g. investigation vs. implementation vs. review, or a domain bucket). State the category and your reasoning briefly to the user.
2. **Route.** Spawn the worker whose profile fits the category:
   - Understanding / locating code → `explore`
   - Making the change → `coder`
   - Checking a change / claim → `reviewer`
   - Or a named workspace agent specialised for that domain.
3. **Hand off with a self-contained brief** matched to that worker's job.
4. **Synthesize** the result for the user; re-classify follow-ups as they arrive.

## Notes
- When a task spans categories, split it and route each part — don't force one worker to do everything.
- The worker_targets are suggestions; pick the true best handler per item.
