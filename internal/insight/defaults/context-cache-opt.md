---
id: context-cache-opt
name: "Context Cache Optimization"
description: "Find prompt-cache breaks caused by unstable context ordering and propose fixes."
channel: workspace-opt
enabled: true
model: claude-cli
scope: [steps, debug]
prefilter:
  minCount: { cache_break: 2 }
---

# Analysis Instruction

You are given one session that suffered repeated prompt-cache breaks. Cache breaks waste tokens
because the stable prefix has to be re-sent. Diagnose the CAUSE and propose a workspace fix:

- **Volatile content early in the prompt** — a timestamp, counter, or freshly-reordered block
  placed before otherwise-stable context. Propose moving it after the stable prefix.
- **A dynamic context/skill that changes every turn** yet sits in the cached region → propose
  marking it dynamic (uncached) or pinning its position.
- **Non-deterministic ordering** of tools/skills/contexts between turns → propose a stable sort.
- **Oversized eager context** that forces frequent eviction → propose trimming or lazy-loading.

For each cause produce a workspace-opt finding:

- **title**, **rootCause**, **severity** (low|med|high).
- **signature** — stable dedupe key naming the culprit (e.g. `context:clock-header:early`).
- **proposedFix** — the concrete workspace/context change the user can apply.

If cache breaks were unavoidable (genuine content change), say so and return no findings.
