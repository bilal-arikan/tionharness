---
name: "SwarmGo Self-Management"
description: "The self-management tool suite an agent uses to run SwarmGo itself — create/edit agents, flows, schedules, tasks, hooks, MCP servers, skills, workspaces, delegate work to subagents, manage artifacts/memory/secrets/logs and app settings — plus how to activate these load-on-demand tools with activate_tools when a task needs them."
when_to_use: "When you need to create or change SwarmGo entities (agents, flows, schedules, tasks, hooks, MCP servers, skills, workspaces), delegate a sub-task to an isolated worker, store a secret, manage artifacts/memory, read logs, or change app settings — and the matching tool is not active yet"
icon: "🛠️"
color: "#10b981"
access: shared
auto_summary: false
---
# SwarmGo — Self-Management Tools

These tools let an agent operate SwarmGo from the inside: build and edit the same
entities a user would from the UI. They are **gated** (only present when the
workspace has the *Self-management* capability on) and **loaded on demand** — they
do not ship at the start of a turn to keep the prompt lean.

## Activating a tool yourself

When self-management is on, every tool below is listed under **"Available Tools
(load on demand)"** in your prompt — name + one-line summary only, schema not yet
loaded. To use one:

1. Call **`activate_tools`** with the exact tool name(s). Activate everything you
   expect to need for the task in a single call.
2. On your next step the full schema is available — call the tool normally.
3. **`tool_search`** searches the load-on-demand catalog by keyword when you don't
   know the exact name. **`deactivate_tools`** drops tools you no longer need.

> You never wait for permission to activate — activation just loads the schema.
> If a self-management tool (agents/flows/schedules/tasks/hooks/mcp/skills/settings) is
> not in the load-on-demand list at all, the workspace does not have
> self-management enabled (ask the user to turn it on in Settings → Capabilities).
> The vault/session/web tools below (`secret_*`, `list_sessions`, `http_get`) are
> load-on-demand independently of self-management — they appear when their own
> capability (secret vault / cross-session context) is on.

## Tool catalog

**Agents** — `list_agents`, `create_agent`, `update_agent`, `delete_agent`.
Provenance enforced: you can delete only agents you created, never the user's and
never yourself. Deleting an agent cascades: it also removes the agent's sessions,
the schedules bound to it, and the tasks it owns (with their runs) — so delete
deliberately, it is not reversible.

**Agent delegation** —
- `run_subagent` — launch an isolated worker (a built-in profile or an existing
  agent) and get back only the final result, so the sub-task's tool output never
  floods the current context. Supports sync (wait for reply, default) and async
  (detached background run). Multiple calls in one turn run in parallel.
  Gated by the *Delegation* capability (`enableDelegation`).

> Built-in profiles: `explore` (read-only search), `coder` (write/edit code),
> `reviewer` (read-only review). Pass an existing agent's name or ID as `target`
> to use a persistent agent instead of an ephemeral profile.
>
> There is no longer a separate `call_agent`, `spawn_session`, or
> `send_agent_message` tool — `run_subagent` covers all agent-to-agent patterns.

**Flows** — `list_flows`, `get_flow`, `create_flow`, `update_flow`, `delete_flow`,
`run_flow` (drive a flow to completion, recorded in the Activity feed). Load the
`swarmgo-flows` skill for the graph schema before authoring one.

**Schedules (routines)** — `list_schedules`, `create_schedule`, `update_schedule`,
`delete_schedule`. Cron-driven prompts delivered to an agent.

**Tasks (kanban board)** — `list_tasks`, `create_task`, `update_task`, `move_task`,
`delete_task`. The board is passive (no run tool). Read/create/edit/move any task;
delete only tasks you created.

**Hooks** — `list_hooks`, `create_hook`, `delete_hook`. PreToolUse/PostToolUse
external commands that intercept native tool calls (Claude Code hook contract).
Read/create any; delete only ones you created.

**MCP servers** — `list_mcp_servers`, `create_mcp_server`, `toggle_mcp_server`,
`delete_mcp_server`. Wire up a new external tool source (stdio subprocess or
sse/http endpoint); a new/enabled server's tools appear on your NEXT turn.
Read/create/toggle any; delete only ones you created.

**Workspaces** — `list_workspaces`, `create_workspace`, `rename_workspace`,
`delete_workspace`. Manage the fully-isolated workspaces (each its own
agents/sessions/flows/secrets) the switcher hops between. List/create/rename any;
**delete only workspaces you created** — never a user-made one, never the one
you're running in, never the last remaining one. A new workspace starts empty
(seeded with the blank template); switch to it in the UI to use it.

**Skills** — `create_skill`, `delete_skill`. Author a reusable workspace skill
(markdown instructions other agents load with `use_skill`); created skills appear
in the catalog next turn. Deleting a skill also strips its slug from every agent
that had it selected, so no agent keeps a dangling reference. (`use_skill` itself
is always available.)

**Artifacts** — `list_artifacts`, `delete_artifact` (delete only agent-created).
`create_artifact` / `update_artifact` are always available in chat.

**Memory & logs** — `memory_add` (store a durable fact), `memory_recall` (pull
matching facts on demand — note recall results are also auto-injected into the
prompt each turn, so explicit recall is only for targeted lookups), `read_logs`
(read the app log ring buffer). Both memory tools are load-on-demand — activate
them like the rest of this suite.

**Secrets & sessions & web** — `secret_list` / `secret_get` (read the encrypted
vault), `secret_set` / `secret_delete` (store or remove a credential — write side,
gated by self-management), `list_sessions` (enumerate sibling sessions; a context
block is also pushed automatically), `http_get` (outbound HTTP GET). These are
load-on-demand too: `activate_tools` first.

**Application settings** — `get_settings`, `update_settings` (read and live-apply
the app-wide settings.json). These change config for the WHOLE application — see
the `swarmgo-settings` skill for the full field reference and safety notes.

## Working principles

- **Activate narrowly, up front.** One `activate_tools` call for the whole task
  beats activating tool-by-tool.
- **Respect provenance.** Delete/destructive operations are restricted to the
  entities you created — don't try to work around that.
- **Prefer reading first.** Use the matching `list_*` / `get_*` tool to see
  current state before you create or edit. Never create an entity (agent, flow,
  skill, schedule, hook, MCP server…) without first checking whether one that
  already does the job exists — extend it instead of duplicating. See
  `swarmgo-guide` → "Before you build: discover first".
- **Background work via `run_subagent`:** to start work that runs without
  blocking the current turn, call `run_subagent` with `wait:"async"` (targets
  an existing agent, not a profile). This internally starts a new detached
  session via the same machinery as the old `spawn_session` primitive.
- For a conceptual overview of SwarmGo's pieces, load `swarmgo-guide`.
