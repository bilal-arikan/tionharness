---
id: context-cache-opt
name: "Context Cache Optimization"
description: "Find prompt-cache breaks caused by unstable context ordering and propose fixes."
channel: workspace-opt
enabled: true
model: claude-cli
scope: [steps, debug, cache]
prefilter:
  minCount: { "cache_break:prompt-or-tools-changed": 2 }
---

# Analysis Instruction

You are given one session whose cached prompt PREFIX kept changing between turns
(`cause=prompt-or-tools-changed`), forcing it to be re-sent. Diagnose the CAUSE and propose a
workspace fix.

Note first: with the prompt epoch on (the default) a session's static prefix and tool schemas
are FROZEN at session start, so this cause should be impossible mid-session — drift is held
back until a deliberate adopt. The cache-event list interleaves `epoch` events; a break sitting
next to `created` / `refreshed` / `compaction` / `ttl-cold` is an EXPECTED adopt and must be
ignored. A break with no adopt beside it is the real finding: either the epoch is disabled for
this workspace, or some path is bypassing the snapshot. Report that as the root cause when the
timestamps show it, rather than guessing at content ordering.

Otherwise, the classic causes:

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

Ignore breaks whose cause is `ttl-or-server-eviction` (nothing changed; the prefix simply
cooled) — the `cache-cooling-waste` lens owns those. If the breaks were unavoidable (a genuine
content change, or an adopt you can see in the epoch events), say so and return no findings.
