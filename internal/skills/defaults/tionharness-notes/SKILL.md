---
name: "TionHarness Memory Notes"
description: "How the workspace memory works and how to use it well: the session briefing you receive at start (workspace card, open loops, the notes that reach you, recent digests), the per-turn pulse, the end-of-turn digest, and the five memory tools — remember, record_work, note_search, note_expand, note_correct. Covers the reach/confidence/supersede contract every note carries."
when_to_use: "When you learned something a later session should know (a lesson, a gotcha, a decision and its reasoning), when you finish a piece of work worth handing off, when you need to recall what was decided or learned before, or when a note you read is wrong or outdated."
icon: "🧠"
color: "#8b5cf6"
access: shared
---
# TionHarness — Memory Notes (durable, cross-session)

The workspace has a **memory**: Markdown notes with a frontmatter block, stored next to
the workspace store, written by agents and by the user, readable by both. It is the
place for what would otherwise be re-discovered every session. Full design:
`_Docs/94-FARKINDALIK-VE-NOTLAR.md`.

## What you receive without asking

- **Session briefing** (first turn, frozen for the session): the workspace card (the
  same projection as `get_view workspace`), open loops (questions waiting for a human,
  stuck sessions, failed runs, stale cards), the resumed progress file, the notes that
  **reach** you, recently finished work (session digests) and the other sessions. Its
  last line is a **context meter**: how many bytes it cost and which sections were
  degraded to pointers. A pointer means "the detail exists; fetch it with the named tool".
- **Per-turn context**: your live checklist, the artifacts of this session and — only
  when it changed — a one-line **workspace pulse**. A pulse marked "needs attention"
  (a human waiting, a stuck session, a failed run) is worth a look before you continue.
- **Session digest** (after every turn, for the next session and the user): what this
  session did, in numbers; never written by a model. You do not act on it; the next
  session's briefing lists it.

## The contract every note carries

| Field | Meaning |
|---|---|
| `kind` | `lesson` · `decision` · `work` · `gotcha` · `pattern` · `profile` · `reference` |
| `scope` | Who it reaches: `workspace` (everyone here), `agent` (only you), `project` (sessions in this working directory). Declared when written; a reader never widens it. |
| `confidence` | `verified` (say how, in `verification`) · `inferred` · `unverified`. Be honest; a wrong "verified" misleads every later reader. |
| `supersedes` / `superseded_by` | A correction never overwrites: the old note stays as a dated record and is served only next to its correction. |
| provenance | `source_session`, `source_agent`, `source` are stamped by the runtime, never claimed. |

Notes link with `[[Note title]]` or `[[NOTE12]]`. Archived and private notes are never
served to agents.

## The five tools

- **`remember`** — one durable memory: a lesson or gotcha that cost time, a decision
  with the alternatives rejected, a pattern, a profile, a reference. The test: *would
  this help a later session, mine or another agent's?* A transient status is not a
  note. If a very similar active note exists, your note is filed as its **correction**
  (the old one is kept, marked superseded) instead of a duplicate.
- **`record_work`** — what happened in THIS session: what changed, decisions,
  learned, open items, how it was verified. Use it at a natural stopping point. A
  lesson that would help a different project is a `remember` as well; do both.
- **`note_search`** — AND-matched words over the notes that reach you (title, body,
  tags). An empty result on a fresh workspace is normal; the store fills as sessions
  record. `all: true` searches past your reach.
- **`note_expand`** — one note in full with its links, backlinks and correction chain.
- **`note_correct`** — files a correction of a wrong or outdated note.

## Habits that keep the memory trustworthy

1. **Say what you found.** When a briefing or a search informs a plan, name the notes
   it rests on, anything that argues against the approach, and an explicit "nothing
   recorded on this" when the memory is empty — a finding, not a blank.
2. **Declare reach narrowly.** A dependency quirk is `workspace`; a repo-specific
   path convention is `project`; a habit of yours is `agent`.
3. **Correct, do not delete.** Wrong notes get a `note_correct`; the dated record stays.
4. **Record on events, not on intention.** You just learned something → `remember` now.
   You just finished a unit of work → `record_work` now. "I will note it at the end"
   decays.
5. **Numbers come from the system.** Digests and the workspace card are computed; do
   not restate their numbers from memory — read them.
