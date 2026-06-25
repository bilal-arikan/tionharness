---
name: "SwarmGo Progress Notes"
description: "How to keep durable, cross-session progress for long-running work in SwarmGo: the automatic todo_write progress file (<cwd>/.swarmgo/progress.json) that the next session resumes from, plus the human-readable PROGRESS.md convention you maintain with the file tools. The SwarmGo-native translation of Anthropic's claude-progress.txt + feature_list.json note-taking pattern for long-horizon agents."
when_to_use: "When a task spans more than one session, may be interrupted, or is large enough that you will lose context — and you want the next session (or a different agent) to pick up exactly where you left off. Use it to record what is done, what is in progress, and what is next, so progress survives compaction and restarts."
icon: "🗒️"
color: "#0ea5e9"
access: shared
---
# SwarmGo — Progress Notes (durable, cross-session)

Long tasks outlive a single context window. Anthropic's harness research is blunt
about it: agents that keep **structured notes on disk** — a progress log plus a
checked feature list — can run for hours and hand off cleanly, because the next
session *reads the notes to get up to speed* instead of starting blind. SwarmGo
gives you two complementary layers for this. Use them.

## Layer 1 — `todo_write` is now durable (automatic)

Your working checklist is no longer ephemeral. Every `todo_write` call is
persisted to a progress file tied to the **project working directory**:

- **Location:** `<cwd>/.swarmgo/progress.json` (when the session has a working
  directory), else a per-agent file under the workspace store.
- **Resume:** when a fresh session starts with no checklist of its own, the most
  recent persisted list is injected into your context as a **"Resumed progress"**
  block. That is your cue to continue from where the last session stopped — not to
  restart.

You do not manage this file by hand; just call `todo_write` and keep it honest:

- Mark exactly **one** item `in_progress` while you work it; flip it to
  `completed` the moment it is done and verified.
- `completed` is the equivalent of a feature-list `passes: true`. Do **not**
  delete items to make the list look finished — an unchecked item is information
  the next session needs.
- Each item may carry optional richer fields (Anthropic feature_list style):
  - `category` — a grouping label, e.g. `"functional"`, `"tests"`, `"docs"`.
  - `steps` — verification sub-steps for that item, e.g. how to confirm it passes.

Example call:

```json
{
  "todos": [
    {
      "content": "New-chat button creates a fresh conversation",
      "status": "in_progress",
      "category": "functional",
      "steps": ["Click New Chat", "Verify empty conversation", "Verify it appears in the sidebar"]
    },
    { "content": "Persist draft on reload", "status": "pending", "category": "functional" }
  ]
}
```

## Layer 2 — a human-readable `PROGRESS.md` (you maintain it)

For work a human will also read or that lives in a git repo, keep a short
`PROGRESS.md` in the project root with your file tools (`Read`/`Write`/`Edit`).
This is the `claude-progress.txt` analogue: prose the next session — or the user —
skims first.

At the **start** of a session on an existing project:

1. `Read` `PROGRESS.md` (and skim recent `git log`) to get oriented.
2. Pick the single most valuable unfinished item.

At the **end** of meaningful work:

1. Append a dated entry: what changed, what was verified, what is left, and any
   decision or gotcha the next session must not rediscover.
2. Keep it short — a running log, not an essay.

Suggested shape:

```markdown
# PROGRESS

## 2026-06-25
- DONE: persistent progress file + cross-session resume (go test green)
- NEXT: UI viewer card for the progress file
- NOTE: progress.json is keyed to cwd; falls back to per-agent store when no cwd
```

## How this relates to your other memory

- **Core memory** (`core_memory_replace`/`append`) = who you are / who the user is
  — free-form, persistent persona/human. Not task state.
- **Progress (this skill)** = what is done / in progress / next — structured task
  state, tied to the project.
- **Goal** = the session's single north-star objective.

Keep them distinct: don't dump task checklists into core memory, and don't put
durable user facts in PROGRESS.md.

## Rules of thumb

- One project, one progress file — sequential sessions share it; that is the point.
- Update progress **as you go**, not only at the end — a crash mid-task should still
  leave an accurate file.
- Never silently drop an item; mark it, don't delete it.
- Prefer `todo_write` for the live, machine-tracked checklist; use `PROGRESS.md`
  for the narrative a human reads.
