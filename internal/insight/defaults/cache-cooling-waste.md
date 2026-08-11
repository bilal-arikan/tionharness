---
id: cache-cooling-waste
name: "Prompt Cache Cooling Waste"
description: "Find sessions that repeatedly let the warm prompt cache go cold and propose cadence fixes."
channel: workspace-opt
enabled: true
model: claude-cli
scope: [debug, cache]
prefilter:
  minCount: { "cache_break:ttl-or-server-eviction": 2 }
---

# Analysis Instruction

You are given one session whose prompt cache repeatedly went COLD between turns
(`cause=ttl-or-server-eviction`). This is NOT a prompt-ordering problem — the cached prefix
was byte-identical; nobody used it in time. The warm window is 1 hour from the last call, so
every gap longer than that re-pays the whole prefix.

Diagnose the CADENCE and propose a scheduling/behaviour fix. Look for:

- **A schedule or automation spaced just beyond the TTL** (e.g. a 90-minute cron): every run
  pays a cold prefix. Propose tightening the interval under 1h, or — if the work genuinely is
  occasional — accepting it and instead SHRINKING the static prefix so a cold turn costs less.
- **Long idle stretches inside one long-lived session** where a fresh session per burst would
  cost the same and keep the context smaller.
- **A big static prefix** (many skills / a large persona / a wide tool catalog) that makes each
  cooling expensive. The waste per break is roughly proportional to it, so trimming the eager
  catalog or moving tools to lazy loading reduces the cost of an unavoidable cooldown.
- **Autonomous work bunched into rare wake-ups** that could be batched into one warm run.

Read the `at=` timestamps in the cache-event list to measure the actual gaps — quote them.
The `wasteUsd` figures are the MEASURED avoidable overpay; sum them for the session. Note that
some providers (OpenRouter, claude-cli subscription) report 0 or estimated waste, so a missing
figure does NOT mean the cooling was free — reason from the gaps and `coldTokens` too.

For each cause produce a workspace-opt finding:

- **title**, **rootCause**, **severity** (low|med|high — scale by summed waste and frequency).
- **signature** — stable dedupe key naming the culprit (e.g. `cache:cooling:schedule-90m`).
- **proposedFix** — the concrete schedule/session/prefix change the user can apply.

Ignore breaks whose cause is `model-changed` or `prompt-or-tools-changed`: those are a
different problem and the `context-cache-opt` lens owns them. If the cooling is genuinely
unavoidable (a truly occasional task with a small prefix), say so and return no findings.
