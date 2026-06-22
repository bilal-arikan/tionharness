---
name: "SwarmGo Guide"
description: "Overview of how SwarmGo works — agents, sessions, tasks, flows, schedules, skills, memory and tools — and how the pieces fit together."
when_to_use: "When you need to understand SwarmGo itself, or to orient before using one of its subsystems"
icon: "🗺️"
color: "#6366f1"
access: shared
subskills: [swarmgo-flows, swarmgo-settings, swarmgo-self-management]
---
# SwarmGo — How It Works

SwarmGo is a multi-agent runtime. Everything lives inside a **workspace**: an
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
  `swarmgo-flows` skill for details).
- **Schedules (routines)** — cron-driven prompts delivered to an agent.
- **Memory** — durable facts an agent recalls across sessions. Recall results are
  auto-injected into the prompt each turn; the explicit `memory_recall` tool (and
  `memory_add` when self-management is on) is load-on-demand — `activate_tools` it
  for a targeted lookup. A MemGPT-style **core memory** (re-injected verbatim every
  turn) is organised into **named blocks** — **persona** (about yourself) and
  **human** (about the user) by default, plus any custom blocks the agent defines —
  edited in place with `core_memory_replace`/`core_memory_append` (pass
  `label:"persona"|"human"|…`, default persona). Each block has a **character
  limit**; a write past it is refused so you condense rather than grow context
  unbounded. The **human** block is also kept current automatically: during the
  dream cycle (reflection), durable facts about the user are distilled from the
  journal and merged into it (HA-1, toggleable). When context fills up a turn warns
  you to persist anything important before it is compacted away.
- **Skills** — reusable instruction sets (like this one). Their summaries are
  advertised in the prompt; full bodies load on demand via `use_skill`.
- **MCP servers** — external tool providers attached per workspace.
- **Secrets** — an encrypted per-workspace vault, read via `secret_list` /
  `secret_get` (both load-on-demand — activate them when a task needs a credential).

## How a turn is assembled

1. A **static prefix** (cached): the agent's soul/identity, the tool catalog and
   the **Available Skills** block (skill slugs + summaries only).
2. A **dynamic suffix**: memory recalled for the message plus cross-session
   context, when enabled.

Skills keep the context lean: only summaries sit in the prompt; you pull a full
body with `use_skill` exactly when a task matches it.

## How to get things done

- **Run work now** → start a chat session with the right agent.
- **Track work** → create a task on the board.
- **Automate multi-step / multi-agent work** → build a flow. Load the
  `swarmgo-flows` skill first.
- **Repeat on a schedule** → create a schedule (routine).
- **Persist knowledge** → add a memory.

- **Tune the app** → read or change application-wide settings live with the
  `get_settings` / `update_settings` tools. Load the `swarmgo-settings` skill for
  the full field reference.
- **Manage SwarmGo itself** → create/edit agents, flows, schedules, tasks, hooks,
  MCP servers and skills, spawn parallel workers, store secrets, or manage
  artifacts/memory/logs with the self-management tools. They are loaded on demand
  — `activate_tools` pulls the one you need. Load the `swarmgo-self-management`
  skill for the catalog and the activation workflow.

When a subsystem needs deeper instructions, load the matching skill rather than
guessing — start with `swarmgo-flows` for orchestration, `swarmgo-self-management`
for operating SwarmGo, or `swarmgo-settings` for configuration.

## Before you build: discover first

Never assume a feature, file, entity, or capability is missing. Before you
implement something new — or tell the user "that doesn't exist yet" — verify
against reality first. Re-implementing something that already exists is the most
expensive mistake you can make: it wastes work, creates duplicates, and can
overwrite a working implementation.

So, before building or concluding absence:

1. **Search and read.** Use `grep`/`glob` to find related code, then `read` the
   files that look relevant. For SwarmGo's own entities, use the matching
   `list_*` / `get_*` self-management tool to see current state.
2. **Confirm, don't guess.** Only say a feature is absent after you have actually
   looked for it and found nothing — name what you searched. "I didn't find X
   after grepping for Y and Z" is a verified claim; "X doesn't exist" from memory
   is not.
3. **Prefer extending over rewriting.** If something close already exists, build
   on it (extend, wire in, refactor) instead of starting a parallel version.

This applies to code, configuration, agents, flows, skills, memories — anything
you might otherwise create from scratch.
