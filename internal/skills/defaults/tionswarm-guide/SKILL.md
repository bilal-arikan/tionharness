---
name: "TionSwarm Guide"
description: "Overview of how TionSwarm works — agents, sessions, tasks, flows, schedules, skills and tools — and how the pieces fit together."
when_to_use: "When you need to understand TionSwarm itself, or to orient before using one of its subsystems"
icon: "🗺️"
color: "#6366f1"
access: shared
subskills: [tionswarm-flows, tionswarm-settings, tionswarm-self-management]
---
# TionSwarm — How It Works

TionSwarm is a multi-agent runtime. Everything lives inside a **workspace**: an
isolated unit with its own agents, data store, secrets and settings. Switching
workspaces never leaks content between them.

## Core building blocks

- **Agents** — autonomous entities bound to a provider/model. Each has a soul
  (persona), identity, tool access and per-agent skill selection.
- **Sessions** — conversation threads. Every execution path funnels into a
  session, so chats, task runs, flow runs and scheduled deliveries are all
  viewable as one streamable transcript. `Kind` tags the origin (chat / task /
  flow / schedule).
- **Tasks** — a kanban board. Each task owns one run session.
- **Flows** — multi-step / multi-agent orchestration graphs (see the
  `tionswarm-flows` skill for details).
- **Schedules (routines)** — cron-driven prompts delivered to an agent.
- **Skills** — reusable instruction sets (like this one). Their summaries are
  advertised in the prompt; load a full body on demand with `use_skill`. Some
  skills are deliberately kept OUT of the prompt (on-demand or file-conditional,
  via `paths:`) to save context — discover them with `skill_search <keywords>`. A
  skill may also ship **bundled files** (templates, references); loading it lists
  them so you can `read` them when the task needs them.
- **MCP servers** — external tool providers attached per workspace.
- **Secrets** — an encrypted per-workspace vault, managed via the `secret` tool
  (`action: list|get|set|delete`; load-on-demand — activate it when a task needs a
  credential).

## How a turn is assembled

1. A **static prefix** (cached): the agent's soul/identity, the tool catalog and
   the **Available Skills** block (skill slugs + summaries only).
2. A **dynamic suffix**: cross-session context and the current clock, when enabled.

Skills keep the context lean: only summaries sit in the prompt; you pull a full
body with `use_skill` exactly when a task matches it. If a task seems to need a
skill you don't see listed, call `skill_search` — conditional/on-demand skills are
discoverable but not advertised.

## Rich replies (chat rendering)

The chat UI renders your markdown richly — use it directly in your reply:

- **Mermaid diagrams** — a ```` ```mermaid ```` fenced block placed **inline in
  your message** renders as a themed SVG (flowchart, sequence, state, class, ER,
  gantt, …). When the user asks you to draw or show a diagram, **put the
  ```` ```mermaid ```` block directly in your reply** so it renders in the
  message flow. Do NOT deliver it only as an artifact — an artifact is a saved
  file the user must open in a separate tab, not a diagram shown in the
  conversation. (Saving one as an artifact *in addition* is fine, but the inline
  block is what the user actually sees in chat.) Prefer a small diagram over an
  ASCII sketch; keep one concept per diagram. The rendered block has
  Source/Expand/Copy controls and re-themes with the app.
- **Diffs** — a ```` ```diff ```` block renders as a colored diff view.
- **Code** — fenced blocks get syntax highlighting and a copy button.
- **Images & video** — a single `![alt](path-or-URL)` renders inline (local paths
  served automatically; an image is click-to-zoom, a `.mp4`/`.webm`/… file becomes
  an inline player). Several media each on their own `![alt](path)` line auto-group
  into a gallery. The full media rules (explicit ```` ```gallery ```` block shape,
  artifact-vs-inline decision) live in the `tionswarm-deliverables` skill.
- Standard GFM (tables, task lists, headings) renders too.

While a reply streams, an incomplete mermaid block shows its source until the
syntax is complete, then swaps to the diagram — so partial output never breaks.
For a non-trivial diagram, lint it first with the **`mermaid_validate`** tool
(load-on-demand) — it catches a bad diagram type or unbalanced brackets/quotes
before the broken block reaches the user.

## Reaching the user (interaction tools)

These tools surface in the TionSwarm UI on every interactive chat turn (both native
and claude-cli agents). They are always available — no `activate_tools` needed.

- **`ask_user`** — pause and ask a clarifying question with optional clickable
  options; **blocks** until the user answers. Use when you genuinely cannot
  proceed without their input.
- **`request_confirmation`** — ask a yes/no before a risky/irreversible action
  (delete, spend, send externally); **blocks**, returns `confirmed`/`denied`.
- **`notify`** — raise a non-blocking desktop notification (`title`, `body`,
  `level: info|success|error`). Use to *inform* when the user may not be looking
  (a long job finished, an autonomous run needs attention). Does NOT ask — it
  only tells; reach for `ask_user` when you need an answer.
- **`focus_view`** — drive the UI to a screen to direct attention (`view` =
  chat/board/flows/artifacts/agents/…; optional `sessionId`/`agentId`). Use to
  *show* ("open the artifact I just made"), not to ask. Non-blocking.
- **`update_session`** — manage THIS session in one call (pass only the fields you
  change): `title` renames it (a clear sidebar label once the topic is known);
  `working_dir` sets the cwd for the file/shell tools (like `cd`; empty string
  resets to the workspace default, takes effect next turn); `tags` / `add` /
  `remove` edit its tags; `archive: true` retires it when the work is done (it
  leaves the active list, never deleted). Track multi-step work with `todo_write`.
  Non-blocking.

On autonomous (scheduler/spawn/flow) turns there is no live user: the blocking
tools (`ask_user`/`request_confirmation`) are withdrawn, while `notify`/
`focus_view` stay available as no-ops when no window is open.

## How to get things done

- **Run work now** → start a chat session with the right agent.
- **Track work** → create a task on the board.
- **Automate multi-step / multi-agent work** → build a flow. Load the
  `tionswarm-flows` skill first.
- **Repeat on a schedule** → create a schedule (routine).

- **Tune the app** → read or change application-wide settings live with the
  `get_settings` / `update_settings` tools. Load the `tionswarm-settings` skill for
  the full field reference.
- **Manage TionSwarm itself** → create/edit agents, flows, schedules, tasks, hooks,
  MCP servers and skills, spawn parallel workers, store secrets, or manage
  artifacts/logs with the self-management tools. They are loaded on demand
  — `activate_tools` pulls the one you need. Load the `tionswarm-self-management`
  skill for the catalog and the activation workflow.

When a subsystem needs deeper instructions, load the matching skill rather than
guessing — start with `tionswarm-flows` for orchestration, `tionswarm-self-management`
for operating TionSwarm, or `tionswarm-settings` for configuration.

## Before you build: discover first

Never assume a feature, file, entity, or capability is missing. Before you
implement something new — or tell the user "that doesn't exist yet" — verify
against reality first. Re-implementing something that already exists is the most
expensive mistake you can make: it wastes work, creates duplicates, and can
overwrite a working implementation.

So, before building or concluding absence:

1. **Search and read.** Use `grep`/`glob` to find related code, then `read` the
   files that look relevant. For TionSwarm's own entities, use the matching
   `list_*` / `get_*` self-management tool to see current state.
2. **Confirm, don't guess.** Only say a feature is absent after you have actually
   looked for it and found nothing — name what you searched. "I didn't find X
   after grepping for Y and Z" is a verified claim; "X doesn't exist" from memory
   is not.
3. **Prefer extending over rewriting.** If something close already exists, build
   on it (extend, wire in, refactor) instead of starting a parallel version.

This applies to code, configuration, agents, flows, skills — anything
you might otherwise create from scratch.
