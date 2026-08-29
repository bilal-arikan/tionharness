---
name: "TionHarness Insight Scanning"
description: "Drive the retrospective session scanner (Insight, _Docs/60) from a chat instruction: run a scan over past sessions through editable lenses, review the deduplicated findings routed to app-fix (bugs in TionHarness itself) or workspace-opt (things to optimize here), present them to the user, and — only on the user's decision — triage each finding. Tools: insight_scan, insight_list_findings, insight_apply_finding."
when_to_use: "When the user asks you to run an insight/retrospective scan, review past sessions for recurring problems (tool errors, wasted tool/skill usage, context/cache issues, lessons), or act on insight findings. Also when a schedule fires this instruction to run the scan automatically."
icon: "💡"
color: "#f59e0b"
access: shared
auto_summary: false
---
# TionHarness — Insight Scanning

Insight scans this workspace's **past sessions** through editable **lenses** and
distils recurring problems into deduplicated **findings**. It is the retrospective,
fleet-wide sibling of the per-turn lessons memory. Full design: `_Docs/60`.

You drive it with three tools. The human stays in the loop: **you scan and report,
the user decides what to act on.** Never mass-apply findings on your own.

## The two channels

- **app-fix** — a bug or gap in **TionHarness itself** (e.g. a tool that misbehaves,
  a disabled tool still advertised to the model). These feed TionHarness development;
  they also append to the configured repo's `_Docs/INSIGHT-BACKLOG.md`.
- **workspace-opt** — something to optimize **inside this workspace** (a skill to
  add, a tool to disable, a CLAUDE.md note, a prefilter to tune). These append to
  `insight/WORKSPACE-ACTIONS.md`.

Findings are **advisory**: applying one is a human/agent decision, not an automatic
mutation. `lessons-mining` findings are the exception — they are promoted into the
lessons store automatically and ride future turns.

## The workflow

1. **Scan.** Call `insight_scan`. Omit `lensIds` to run all enabled lenses, or pass
   specific ones (e.g. `["tool-errors"]`). Scans are **incremental** — a session
   already scanned by a lens is skipped unless it changed, so re-running is cheap.
   The first full scan of a large workspace can take minutes (one LLM call per
   (lens,session) pair); later scans are fast.
2. **Review.** Call `insight_list_findings` (filter `status:"new"` for untriaged
   ones, or a `channel`/`lens`). Each line begins with the finding **id**.
3. **Report and STOP.** Summarize for the user, grouped by channel, leading with
   **high-severity** and **recurring** (`×N`) findings. Then **ask what to do** —
   do not triage on your own.
4. **Act on the user's decision only.** For each finding the user chooses, call
   `insight_apply_finding` with the id and a status:
   - `accepted` — the user will act on it.
   - `applied` — the proposed fix has been done.
   - `verified` — the fix is confirmed effective.
   - `dismissed` — not worth acting on.
   This records the decision; it does **not** mutate the workspace. If the user
   asks you to actually *implement* a fix, do that with your normal tools (edit a
   skill, a CLAUDE.md note, app code) and then mark the finding `applied`.

   `applied` requires **evidence**: pass `evidence` with the `entityType` and
   `entityId` you changed (e.g. `{"entityType":"skill","entityId":"tionharness-insight"}`).
   Without it the call fails — a decision you did not act on is `accepted` or
   `dismissed`, never `applied`.

   To close several findings at once, pass `ids` (a list), or pass one `id` with
   `applyCluster: true` to also close the near-duplicates clustered with it.
   `insight_list_findings` with `cluster:true, verbose:true` prints those member ids.

## Automation

**A scan session is tagged `insight-scan`** and fires the turn-finished signal when it
completes, so tag automations can react to it. One such rule ships with every workspace:
`insight-apply-workspace-opt` targets the `insight-applier` system agent, which applies the
`workspace-opt` findings (`status:"new"`) to workspace entities — a skill, an agent, a hook,
an automation — and marks each one `applied`. It never touches the `app-fix` channel and it
cannot reach repo files (no file or config tools in its allowlist). The rule ships
**disabled**: the user enables it in the Automations screen when they want findings applied
without being asked each time.

To run this on a schedule, create a schedule (self-management `create_schedule`)
whose prompt is a scan-and-report instruction, e.g. *"Run an insight scan over new
sessions and summarize the important findings by channel; do not apply anything."*
The scheduled turn will scan and present the results in its session for later
review. **Even when automated, do not auto-apply** unless the user's instruction
explicitly authorizes a specific, safe action.

## Tips

- Unscoped scans consider at most 10 sessions by default. Pass `maxSessions` or
  change the workspace Insight setting when a different budget is required;
  explicit `0` means unlimited.
- Findings come **priority-ranked** (severity × recurrence). A `⚠REGRESSED` finding is a closed
  issue that came back — treat it as top priority.
- When a scan surfaces many similar findings, list with `cluster:true` to collapse near-duplicates
  into one representative + a count, then triage the cluster, not each variant.
- Prefer `status:"new"` when reviewing so you don't re-surface already-triaged items.
- A finding's `×N` count is how many sessions it recurred in — higher = more worth fixing.
- If a scan reports errors (e.g. an unauthenticated claude-home for the analysis
  agent), report them; the affected (lens,session) pairs stay retryable next scan.
