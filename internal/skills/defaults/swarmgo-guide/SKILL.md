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
  (persona), identity, tool access, optional heartbeat (autonomous wake) and
  per-agent skill selection.
- **Sessions** — conversation threads. Every execution path funnels into a
  session, so chats, task runs, flow runs and scheduled deliveries are all
  viewable as one streamable transcript. `Kind` tags the origin (chat / task /
  flow / schedule / heartbeat).
- **Tasks** — a kanban board. Each task owns one run session.
- **Flows** — multi-step / multi-agent orchestration graphs (see the
  `swarmgo-flows` skill for details).
- **Schedules (routines)** — cron-driven prompts delivered to an agent.
- **Memory** — durable facts an agent recalls across sessions. Recall results are
  auto-injected into the prompt each turn; the explicit `memory_recall` tool (and
  `memory_add` when self-management is on) is load-on-demand — `activate_tools` it
  for a targeted lookup.
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
