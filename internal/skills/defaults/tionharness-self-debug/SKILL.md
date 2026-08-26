---
name: "TionHarness Self-Debug"
description: "How to read your own per-session debug journal in TionHarness to self-diagnose and optimise: the read_session_debug tool over the parallel debug.jsonl observability stream (turn timings, per-call token spend by model, per-tool latency/size/errors, hook decisions, compaction and recovery). Use it to find where tokens and time go, which tools are slow or failing, and how often context is compacted — then change behaviour."
when_to_use: "When a session feels slow, expensive, or error-prone, or when you are explicitly asked to optimise token/latency usage or investigate why a run misbehaved. Also useful at the end of a long autonomous run to review cost/latency and record a lesson. Read the summary first; drill into raw events only when a number looks wrong."
icon: "🐞"
color: "#ef4444"
access: shared
---
# TionHarness — Self-Debug (read your own observability stream)

Every session has a **parallel debug journal** next to its conversation:
`store/sessions/<id>/debug.jsonl`. Where `session.jsonl` is what the user sees,
`debug.jsonl` is structured **observability** the runtime writes for you — so you
can look at your own behaviour and improve it. You read it with one tool:
**`read_session_debug`**.

## What is recorded

Each event is one of:

- **`turn`** — one assistant turn finished: duration (`durMs`), stop reason, error flag.
- **`llm_call`** — one provider completion: `model`, input/output/cache tokens.
- **`tool`** — one tool execution: `name`, `durMs`, output size (`outBytes`), error flag.
- **`hook`** — a PreToolUse/PostToolUse hook ran: which tool, the decision (allow/block/modify).
- **`error`** — a turn-level / permission / budget error.
- **`compaction`** — in-flight history was compacted (context pressure).
- **`recovery`** — a turn recovery fired (output-cap resume, or compact-and-retry).

## How to read it

Default to the **summary** — the aggregate is what you read first:

```json
// read_session_debug  (summary is the default)
{ "summary": true }
```

It returns totals (turns, llm calls, tool calls, input/output/cache tokens),
health counters (`errors`, `compactions`, `recoveries`), `byTool` (calls / errors
/ total duration / bytes per tool), `byModel` (tokens per model), `topTools` (the
slowest tools — your optimisation hot list), and `lastError`. It also returns:

- **`anomalies`** — heuristic findings already computed for you: a tool dominating
  tool time, a tool failing often, oversized tool outputs, frequent compaction, an
  error burst, slow turns. Start here — each anomaly is a concrete thing to fix.
- **`turnDurSeries` / `tokenSeries`** — recent per-turn duration and per-call token
  trends (for spotting a regression over the session).

Reading these `anomalies` yourself mid-session lets you act *now* — turn a
recurring finding ("batch Bash calls", "ask Read for less") into a lesson you
record in the progress file so the next session starts already knowing it.

Drill into raw events only when a number looks wrong:

```json
// newest 50 tool events, raw
{ "summary": false, "type": "tool", "limit": 50 }
```

Pass `session_id` to inspect a different session (e.g. a subagent's). With no
`session_id` it reads the session you are running in.

## Never grep the store raw

The session store (`<workspace>/store/sessions/**`) is JSONL where **one line is
one whole message**, `steps` payload included — routinely tens or hundreds of KB
per line. `rg -n` prints the entire matching line, so a two-word search over the
store can return a megabyte from twenty matches, and that megabyte lands in your
context. This has really happened.

- Search conversations with the **`conversation_search`** tool, not `rg`.
- If you must touch the files, never print whole lines: use `rg -o`, add
  `--max-columns 200`, or parse fields with `python -c`.
- Exclude the store explicitly when searching a workspace directory
  (`--glob '!store/sessions/**'`) — and write the command in **Git Bash**, since
  PowerShell quoting silently mangles `--glob` patterns and the exclusion is then
  never applied.

## Turning numbers into action

- **High `cacheRead` is good** — prompt cache is working. **Low cache + high
  `input`** across many `llm_call`s means the prefix keeps changing; avoid
  rewriting early context.
- **A tool dominating `topTools` by duration** → batch its calls, narrow its
  inputs, or replace it (e.g. one targeted `Grep` instead of many `Read`s).
- **Large `outBytes` on a tool** → the result is bloating context; ask for less
  (filters, head limits) so compaction doesn't have to trim it.
- **`compactions` climbing** → you are running hot on context; persist important
  facts to the **progress** file before they are folded away, or hand off.
- **Repeated `error` of the same kind** → fix the cause, don't retry blindly;
  `lastError` names the most recent one.

## Where this fits

- **`read_logs`** = the process-wide log stream (cross-session, slog). Coarse.
- **`read_session_debug`** (this skill) = *your* session's structured metrics. Precise.
- **Per-session budget** (UI / usage-detail) = lifetime cost rollup. Money, not mechanics.

Read the debug summary, find the one biggest cost or failure, change one thing,
and — for a durable lesson — record it in `PROGRESS.md` so the next
session starts already knowing it.
