---
name: "TionSwarm Autonomous Ops"
description: "How to reach the 'expert' level of agentic operation in TionSwarm: stop hand-prompting and review-looping, and instead encode repeatable work as skills, fire it on triggers with schedules and hooks, let goal-bounded autonomous agents loop until done, run work in parallel across isolated workspaces, mix models per task, and guard it all with the autonomy brake and the permission layer. The TionSwarm-native translation of the expert 'vibe coding' automation playbook."
when_to_use: "When a user (or you) keeps doing the same prompt/review cycle by hand and wants to automate it — set up recurring sweeps (docs/tests/logs), event-driven reactions (hooks), long-running goal loops, parallel fan-out, or a multi-model build→write→review pipeline — and needs to know which TionSwarm primitive maps to which automation pattern."
icon: "♻️"
color: "#6366f1"
access: shared
---
# TionSwarm — Autonomous Ops Playbook

There are levels to agentic work. **Beginners** prompt, wait, review, prompt
again — a human in every loop. **Experts** encode the loop once and let the
system run it: reusable skills, triggered automations, goal-bounded loops,
parallel fan-out, and the right model for each step. This skill is the
TionSwarm-native translation of that playbook — every pattern below maps to a real
TionSwarm primitive, not an aspiration.

> Reference context (what TionSwarm *is*): load `tionswarm-guide`.
> The tools that *do* these things: load `tionswarm-self-management`.
> Flow graph schema: load `tionswarm-flows`. App settings: `tionswarm-settings`.

## Concept map (playbook → TionSwarm)

| Expert pattern | TionSwarm primitive |
|----------------|-------------------|
| Coding agents (multiple harnesses) | **Agents** over providers: `claude-cli` (keyless), `anthropic`, `minimax`/OpenAI-compat, + custom providers |
| `agents.md` / `CLAUDE.md` rules | **Workspace config** (editable prompt/instructions) + per-agent system prompt + `tionswarm-settings` |
| Skills ("anything done twice") | **File-based skills**: `create_skill` / `use_skill`, global+workspace tiers, auto-summary |
| Automations (trigger → prompt) | **Schedules** (cron, workspace-scoped prompt delivery) for time triggers; **Hooks** (PreToolUse/PostToolUse) for event triggers |
| Loops (run until goal) | **Schedules** (cron), **`schedule_wake`** (single-shot self-wake, interactive turn only), **`run_subagent` async** (detached background run), and **Flows** (graph engine) — all gated by the per-workspace autonomy brake |
| Quality gates | **Permission/approval layer** (`auto`/`ask`/`read-only`, arg-patterns) + **Hooks** |
| Auto code review (e.g. Greptile) | A dedicated **reviewer agent** invoked by a flow or schedule |
| Cloud vs local / infinite parallel | **Physical workspace isolation** + `run_subagent` (parallel isolated workers — sync or async) |
| Git worktrees (avoid conflicts) | **Per-workspace `store/` isolation** (the closest analog; see limits below) |
| Multimodal (model per task) | **Per-agent provider/model** + a **flow** whose nodes use different agents/models |
| Flywheel: perfect tests/docs/logs | Recurring **schedules** that sweep docs, tests, and `read_logs` nightly |

## 1. Agents & providers — pick the harness per job

TionSwarm runs each agent as its own runtime over a provider. You are not locked to
one vendor:

- **`claude-cli`** — local Claude CLI, keyless (uses the machine login). Drives its
  own tool loop via MCP delegation.
- **`anthropic`** — Messages API (native tool-use loop, token streaming, thinking).
- **`minimax` / OpenAI-compat** — any OpenAI-shaped endpoint via base URL.
- **Custom providers** — OpenRouter / Gemini / Kimi / Ollama, added in settings.

**Why mix them (speed + cost):** you do not need a frontier model for every step.
Reserve the strongest model for planning/review and use a cheaper, fast model for
the bulk code-writing. Set the model per agent; see the multi-model pipeline in §8.

## 2. Rules — the `agents.md` analog

Define *how* you want agents to behave once, not in every prompt:

- **Per-agent system prompt / instructions** — personality, response length
  ("short and sweet, no essays"), coding conventions, commit style.
- **Workspace config files** — editable prompt/instructions that apply to the
  whole workspace.
- **`tionswarm-settings`** — app-wide behavior an agent can read/live-apply with
  `get_settings` / `update_settings`.

Start with the model's voice and your hard preferences; refine as you learn what
you keep correcting by hand — then bake that correction into the rules.

## 3. Skills — anything you do more than once

The single highest-leverage habit: **if you do it twice, make it a skill.**
A skill is markdown instructions (optionally with reference files) that any agent
loads with `use_skill`. Author them with `create_skill` (self-management).

Use skills for:

1. **Repeated prompts** — invoke instead of re-pasting.
2. **Domain rules** — house writing style, issue templates, company facts.
3. **Tool/CLI instructions** — how tests are kicked off, how a specific API/CLI is
   called, expected responses. Define once; never re-explain the endpoints.
4. **Quality gates** — "before opening a PR run all tests, require 100% pass, fix
   failures" encoded as one invokable procedure.

Agents **auto-discover** relevant skills at runtime (auto-summary surfaces them),
so you do not always have to name one explicitly. Keep skills focused and
composable rather than one mega-skill.

## 4. Automations — triggers that prompt an agent

Three trigger families — pick by what fires the work:

### Time triggers → Schedules
Cron-driven prompts delivered to an agent, scoped to a workspace
(`create_schedule`). This is the engine behind every nightly "sweep". Example
intent: *"At 01:00 daily, review the codebase, update any stale docs, and report
what changed."*

### Event triggers → Automations (the `Automation` entity)
The richer, event-driven complement to a cron Schedule: `create_automation` fires a
target agent **or** flow when a domain EVENT happens — a **tagged session finishing**
a turn, a **kanban card changing**, cumulative **token spend** crossing a threshold,
or a session's **message/tool count** crossing an interval. It carries its own
guardrails (maxIterations/cooldown/expiry), a spawn-vs-continue **session mode**, and
a bounded tag **self-loop**. This is how you build "when a card enters Review, run the
reviewer" or "every 150k tokens, run a maintenance pass" without a clock.

> Full trigger kinds, config surface, session mode, and recipes: load
> **`tionswarm-automations`**. (Do not confuse these with cron Schedules — an
> Automation reacts to events, not time.)

### Tool-call triggers → Hooks
`PreToolUse` / `PostToolUse` external commands intercept native tool calls (Claude
Code hook contract) — `create_hook`. Use them to *gate* (block a risky tool before
it runs) or *react* (lint/format/log after a write). This is your tool-level
automation + part of your quality gate.

> The video's "PR opened → wait for review comments → address → push" automation
> translates to: a **schedule or flow** that triggers a **reviewer agent**, which
> waits for / reads the review output and then has the worker agent apply fixes.

## 5. Loops — run until a goal is met

A loop is just **trigger + repeated action + stopping goal** (so it does not run
forever). TionSwarm gives you these options — there is no longer a periodic
heartbeat ticker; every autonomous run requires an explicit trigger:

- **Schedules** — a cron schedule re-invokes an agent on a timer. Give it a
  goal-shaped prompt; each tick iterates toward the goal. Always give the prompt a
  concrete stopping goal — daily spend caps were removed (agents are unlimited), so
  the **per-workspace `pauseAutonomy` brake** (toggled on the Schedules screen) is
  the one hard stop for a runaway loop.
- **`schedule_wake`** — a single-shot self-wake timer inside an interactive chat
  turn; the agent fires once more after the delay. Good for "check back in N
  minutes" within a conversation, not for standing automation.
- **`run_subagent` (async)** — kick off a detached background run for an existing
  agent (`wait:"async"`); the current turn proceeds immediately. The subagent runs
  one autonomous turn; re-invoke with another async call if the loop needs to
  iterate.
- **Flows** — the graph engine (`agent`/`branch`/`parallel`/`delay`/`transform`)
  for a *structured* loop with explicit branch conditions and restart-safe state.

**Concrete loops to ship (schedule the autonomous variant):**

```mermaid
graph LR
    A[Overnight Docs Sweep] --> A1[Nightly: diff yesterday's changes<br/>vs docs, update gaps, report]
    B[Quality Loop] --> B1[Walk the app/codebase,<br/>fix until the bar is met]
    C[Production Error Sweep] --> C1[Nightly: read_logs, find errors,<br/>diagnose, write a fix, report]
```

- **Overnight docs sweep** — schedule at 01:00 → agent compares the day's changes
  to docs and closes gaps.
- **Quality loop** — autonomous agent keeps optimizing until a measurable bar
  (test pass %, perf target) is met, then stops.
- **Production error sweep** — schedule → agent calls `read_logs`, analyzes each
  error, proposes a fix. "By morning the fix is already written."

## 6. Parallelism — fan out without melting the machine

- **`run_subagent` (multiple calls in one turn)** — each `run_subagent` call in
  a single turn runs in parallel (goroutine fan-out, shared atomic budget). Use
  built-in profiles (`explore`/`coder`/`reviewer`) for ephemeral isolated workers,
  or pass an existing agent name/ID for a persistent agent. Always installed
  (disable per-agent from the Tools screen). Concurrency is bounded by
  `SpawnMaxConcurrent` and `DelegationMaxCalls`.
- **Physical workspace isolation** — each workspace is a separate `store/` +
  runtime + scheduler. This is TionSwarm's "isolated environment" answer to the
  cloud-agent pitch: agents in different workspaces never collide.

**The worktree caveat (be honest):** the expert playbook uses git worktrees so
parallel agents don't clobber the same files. TionSwarm isolates at the
*workspace* level, not per-agent-within-a-workspace. So multiple agents writing
the **same files in the same workspace** can still conflict — split them across
workspaces, or give each a non-overlapping area of the codebase.

> TionSwarm has no built-in git merge/deploy orchestration — the "many agents
> racing to merge into main" problem from the playbook is out of scope here. If
> agents touch a real git repo, serialize merges yourself or batch them.

## 7. The quality flywheel

The expert claim: there is no excuse for sub-optimal code, stale docs, or blind
spots — because each can be a standing automation:

1. **Tests** — a schedule that checks coverage and writes missing tests.
2. **Docs** — the overnight docs sweep (§5).
3. **Logging** — "log everything" with a 7–30 day window, then let the production
   error sweep mine it. Full log coverage is what makes the error loop possible.

Three standing loops — tests, docs, logs — compound into a codebase that stays
healthy without you in every cycle.

## 8. Multi-model pipeline (build → write → review)

Encode model selection as a **skill + flow** so each step uses the right tool:

```mermaid
graph LR
    P[Plan<br/>strongest model] --> W[Write code<br/>fast/cheap model]
    W --> R[Review<br/>different model]
```

- **Plan** with your most capable model (sees the whole codebase, designs the
  change).
- **Write** with a fast, cheaper code model — plan is done, raw execution doesn't
  need the frontier model.
- **Review** with a *different* model than wrote it, for an independent viewpoint.

In TionSwarm: distinct agents (each pinned to its provider/model) wired as nodes in
a flow, or a skill that says which agent to hand off to at each stage.

## 9. Guardrails — keep autonomy safe

- **Autonomy brake (per-workspace)** — pausing autonomy (Schedules screen,
  `pauseAutonomy` in `ws-settings.json`) stops that workspace's autonomous provider
  calls at the single `guardedComplete` funnel; every call is usage-metered.
- **Permission layer** — `auto` / `ask` / `read-only` modes; arg-based patterns
  narrow a blanket "always allow" to a command family (e.g. `shell(git *)` runs
  unattended while `rm` still prompts). Run loops in a mode you trust.
- **Hooks as hard gates** — a `PreToolUse` hook can block a tool before it runs.
- **Provenance** — destructive self-management ops are limited to entities the
  agent created. Lean on it; don't work around it.

## 10. The autonomous boot sequence (run this first, every time)

A scheduled / spawned / flow turn starts with **no memory of the last run** — the
context window is fresh. Before touching any code, walk this fixed startup routine
so a lost context never means lost orientation (the long-running-agent harness
discipline). It is the autonomous mirror of a human opening the project each morning.
Headless turns get a short reminder of this sequence injected automatically (gated by
the `autonomousBootSeq` setting, default on); this section is the full recipe behind it.

> One turn = **one task**. Orient, verify, do exactly one unit of work, then close
> the loop. Do not batch many tasks into a single autonomous turn.

### Step 0 — Orient
- Confirm where you are: `Bash` → `pwd` (or the cwd badge / `Session.WorkingDir`),
  and the git branch + dirty state (`git status -sb`).
- Note your file scope: built-in fs/shell tools reach the whole machine, but
  `autonomousConfine` (default on) keeps writes inside the working dir and blocks
  `git push`. Stay inside the working dir.

### Step 1 — Recall (git + progress file + board are your memory)
- `Bash` → `git log --oneline -15` — what shipped recently and in what state.
- Read the **persisted progress file** if the workspace keeps one
  (`<cwd>/.tionswarm/progress.json`, written when `progressPersist` is on — TionSwarm's
  `claude-progress` analog); it records what the previous turn left half-done. With
  `progressResume` on, a fresh session already gets this injected.
- `list_tasks` — the append-only Kanban board is the workspace's feature/work ledger.
  Treat task notes as the canonical "what's left" record.

### Step 2 — Select ONE task
- Pick the **single highest-priority unfinished item**: the lowest-numbered `todo`
  on the board (or the explicit goal in the schedule prompt). Move it to
  `in_progress` (append-only: never rewrite a title or delete a prior note).

### Step 3 — Verify the baseline BEFORE you build
- Run the project's smoke / e2e check first — a `Bash` test command, or a
  hand-testable **flow** (`run_flow`). This catches an *undocumented* broken state
  the previous turn may have left behind.
- **If the baseline is red, that broken state IS this turn's task.** Fix it (or
  revert the offending commit — `git` is your undo), re-verify green, then stop.
  A clean baseline is worth more than a half-built feature on a broken tree.

### Step 4 — Do the one task
- Implement only the selected unit. Stay within the permission mode you were
  launched in (per-agent spend caps were removed — agents are unlimited).

### Step 5 — Close the loop (leave a clean handoff)
- Re-run the baseline check; require green before you finish.
- `git commit` with a descriptive message (a clean commit = the next turn's
  recoverable state). Remember `autonomousConfine` blocks `git push` — commit locally.
- **Append** an outcome note to the board task (and the progress file): what changed,
  what's still open. Append-only — never overwrite the prior note.
- Move the task to `done` only if it is fully verified; otherwise leave it
  `in_progress` with a note on where it stands.

> Why this order matters: orientation + baseline-first is what lets autonomous turns
> compound instead of drift. Skipping the verify step is the single most common way a
> long-running loop silently builds on top of a broken tree.

## Putting it together (a reference setup)

0. On every autonomous turn, run the **boot sequence** (§10) before anything else.
1. Write the **rules** (per-agent instructions) so every agent talks/codes your way.
2. Turn each repeated procedure into a **skill** (tests gate, deploy checklist, API recipe).
3. Add standing **schedules**: nightly docs sweep, coverage check, production error sweep.
4. Add **hooks** for event reactions (format-on-write) and hard gates (block destructive tools).
5. For big jobs, **fan out with `run_subagent`** (parallel profiles or agents) and/or split across **workspaces**.
6. Build the **multi-model flow** for feature work (plan → write → review).
7. Cap everything with the **autonomy brake + permission mode** so it runs unattended safely.

## Pitfalls

- **Loop with no goal** = runaway. Always state the stopping condition; rely on the
  per-workspace autonomy brake as a backstop, not the plan.
- **Same-workspace file conflicts** — parallel agents on the same files still
  collide; isolate by workspace or by code area.
- **`run_subagent` fan-out is capped** — `DelegationMaxCalls` (default 8) per turn and `SpawnMaxConcurrent` (default 16) overall. Design fan-out within these limits.
- **MCP/server changes apply next turn** — a newly created server's tools aren't
  available until the following turn.
- **No git merge/deploy layer** — handle real-repo merges outside TionSwarm.
- **Skipping the boot sequence** — an autonomous turn that dives straight into code
  without orient → recall → baseline-verify will eventually build on a broken tree.
  Always run §10 first.
