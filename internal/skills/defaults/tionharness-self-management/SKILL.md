---
name: "TionHarness Self-Management"
description: "The self-management tool suite an agent uses to run TionHarness itself — create/edit agents, flows, schedules, tasks, hooks, MCP servers, skills, workspaces, delegate work to subagents, manage artifacts/secrets/logs and app settings — plus how to activate these load-on-demand tools with activate_tools when a task needs them."
when_to_use: "When you need to create or change TionHarness entities (agents, flows, schedules, tasks, hooks, MCP servers, skills, workspaces), delegate a sub-task to an isolated worker, store a secret, manage artifacts, read logs, or change app settings — and the matching tool is not active yet"
icon: "🛠️"
color: "#10b981"
access: shared
auto_summary: false
---
# TionHarness — Self-Management Tools

These tools let an agent operate TionHarness from the inside: build and edit the same
entities a user would from the UI. They are always built; visibility is set **per
tool** on the Tools ("Araçlar") screen (full / summary / name-only / hidden), and
they are **loaded on demand** — they do not ship at the start of a turn to keep the
prompt lean.

**This skill IS their catalog.** To save context, the self-management tools are NOT
listed individually in your system prompt's "Available Tools (load on demand)"
block — that block shows only a one-line pointer here. The full list of names lives
in the **Tool catalog** section below; read it, pick the exact names you need, then
activate them.

## Activating a tool yourself

The self-management tools are registered but their names+schemas are not in your
prompt. To use one:

1. Find its exact name in the **Tool catalog** below (or call **`tool_search`** with
   a keyword — it searches the on-demand catalog including these hidden tools).
2. Call **`activate_tools`** with the exact tool name(s). Activate everything you
   expect to need for the task in a single call.
3. On your next step the full schema is available — call the tool normally.
4. **`deactivate_tools`** drops tools you no longer need.

> You never wait for permission to activate — activation just loads the schema.
> If `activate_tools` reports a self-management name as unknown, check the spelling
> against this catalog (or `tool_search`); if it is right, the tool is disabled for
> this agent on the Tools screen — ask the user to enable it there. There is no
> workspace-wide self-management switch any more. The vault/session tools below
> (`secret`, `list_sessions`, `conversation_search`) are load-on-demand too and show
> in the prompt's load-on-demand block (not hidden).

## Tool catalog

**Agents** — `list_agents`, `create_agent`, `update_agent`, `delete_agent`.
`create_agent` takes an optional `skills` array (slugs); omit it and the new agent
is seeded with the default TionHarness skill set, unknown slugs are skipped.
Provenance enforced: you can delete only agents you created, never the user's and
never yourself. Deleting an agent cascades: it also removes the agent's sessions,
the schedules bound to it, and the tasks it owns (with their runs) — so delete
deliberately, it is not reversible.
System agents cannot be deleted (`delete_agent` returns HTTP 409). Disable one to
use its built-in fallback, or use restore-default to replace its editable profile
fields with the compiled defaults.

**Agent delegation & messaging** —
- `run_subagent` — launch an isolated worker (a built-in profile or an existing
  agent) and get back only the final result, so the sub-task's tool output never
  floods the current context. It is always synchronous: the call blocks until the
  subagent finishes and returns its final reply. Persistent execution sessions become `running` only
  after their opening task is durable; initialization failures are terminal and
  remain inspectable in the Activity feed. Multiple calls in one turn run in parallel.
  For sharper delegation, also pass `objective`, `output_format` and `boundaries`
  (all optional) — they are injected as a "Task contract" into the subagent's
  system prompt so it has a clear goal, a required reply shape and explicit scope
  limits (prevents duplicated work and gaps). Always installed (disable per-agent
  from the Tools screen). Use when you need a RESULT back.
- `send_message` — send a direct, addressed message (`{to, message, summary?}`) to
  ANOTHER agent. It lands in that agent's persistent inbox tagged with your name,
  and the agent processes it on its own in the background — you do NOT wait, and
  its reply does NOT return to this conversation (it may message you back, landing
  in YOUR inbox). Your plain reply text is not visible to other agents; to reach
  one you must use this tool. Use for ONGOING peer collaboration (vs `run_subagent`
  for a result-in-this-turn).
- `handoff_session` — **context reset**: when THIS conversation is getting long and
  you are nearing the context limit, write a structured handoff document (objective,
  progress done/pending, environment, next concrete step) as an artifact and continue
  the work in a FRESH session with a clean window — instead of letting older turns be
  silently summarized. The new session runs on its own (activity feed); THIS
  conversation does not continue, so finish your current thought first and do not
  promise more work here. Optional `{reason}`.

> Built-in run_subagent profiles: `explore` (read-only search), `planner` (read-only;
> turns a task into an ordered implementation plan), `coder` (write/edit code),
> `reviewer` (read-only review), `validator` (runs builds/tests/git but never edits
> source; replies with a compact PASS/FAIL verdict), `config` (mini-agent: quick edits to
> config/ files — prompts/instructions/statuses/labels/permissions — sandboxed to
> the config tools). Pass an existing agent's name or ID as `target` to use a
> persistent agent instead of an ephemeral profile.
>
> There is no separate `call_agent` / `spawn_session` / `send_agent_message` tool:
> `run_subagent` covers delegation (isolated task → result) and `send_message`
> covers peer messaging (addressed DM → recipient's inbox).

**Flows** — `list_flows`, `get_flow`, `create_flow`, `update_flow`, `delete_flow`,
`run_flow` (drive a flow to completion, recorded in the Activity feed). Load the
`tionharness-flows` skill for the graph schema before authoring one.

**Schedules (routines)** — `list_schedules`, `create_schedule`, `update_schedule`,
`delete_schedule`, `run_schedule`. Cron-driven prompts delivered to an agent.
`run_schedule` fires a schedule **immediately** ("Run now"), regardless of its
cron timing or enabled state — the manual trigger; it works on any schedule, not
only ones you created (running is not destructive).

**Tasks (kanban board)** — `list_tasks`, `get_task`, `create_task`, `update_task`,
`move_task`, `set_archived_task`, `delete_task`. The board is passive (no run
tool). `list_tasks` returns active cards by default and archived cards separately
with `archived:true`; `get_task` returns one complete card. Read/create/edit/move/
archive/delete ANY task (including user-created ones) — `delete_task` is irreversible.

**Hooks** — `list_hooks`, `create_hook`, `update_hook`, `delete_hook`. External
commands speaking the Claude Code hook contract. Tool events `PreToolUse` /
`PostToolUse` intercept native tool calls (matcher = tool-name glob); lifecycle
events are `UserPromptSubmit`, `SessionStart`, `Stop`, `SubagentStop`,
`PreCompact`, `Notification`, `SessionEnd`. Read/create any; delete only ones you
created.

**Archive (agents, skills, artifacts, automations, goals)** — `set_archived` with
`{kind, id, archived}` (`kind` = `agent`|`skill`|`artifact`|`automation`|`goal`; for a
skill `id` is the slug). A reversible hide: the entity leaves default lists (see it
again with `archived:true` on `list_agents`/`list_artifacts`/`list_automations`) and
`archived:false` restores it. Archived agents cannot run, archived automations do not
fire, archived skills are not offered. Built-in (system) agents cannot be archived.
Restoring a goal returns it to draft. Prefer archiving over `delete_*` when the user
may want it back. Kanban cards use `set_archived_task`.

**MCP servers** — `list_mcp_servers`, `create_mcp_server`, `toggle_mcp_server`,
`delete_mcp_server`. Wire up a new external tool source (stdio subprocess or
sse/http endpoint); a new/enabled server's tools appear on your NEXT turn.
Read/create/toggle any; delete only ones you created. Set the optional
`description` (a short "what it's for / when to use" one-liner) on
`create_mcp_server` — it rides the load-on-demand catalog's per-server summary,
so the semantic hint survives even when the workspace has too many MCP tools to
list individually.

**Workspaces** — `list_workspaces`, `create_workspace`, `rename_workspace`,
`delete_workspace`. Manage the fully-isolated workspaces (each its own
agents/sessions/flows/secrets) the switcher hops between. List/create/rename any;
**delete only workspaces you created** — never a user-made one, never the one
you're running in, never the last remaining one. A new workspace uses the bundled
blank template: it starts with a read-only trigger/observer **CEO**, an execution
lead **PM**, a 20-minute CEO board heartbeat, and enabled PM rules for cards entering
`failed` or `review`. Switch to it in the UI to use it.

**Skills** — `create_skill`, `update_skill`, `delete_skill`, `import_skill`. Author a reusable
workspace skill (markdown instructions other agents load with `use_skill`);
created skills appear in the catalog next turn. `update_skill` edits one in place
by slug — pass only the fields to change (name/description/whenToUse/group/body/shared),
omitted fields keep their current value; prefer it over delete + recreate. `group`
is an organisation label — skills sharing one are folded together in the Skills UI. Only
workspace-tier skills can be edited or deleted (global/bundled are protected).
Deleting a skill also strips its slug from every agent that had it selected, so no
agent keeps a dangling reference. (`use_skill` itself is always available.) After
authoring or editing, run **`skill_validate`** (read-only, load-on-demand) to check
the SKILL.md's slug/frontmatter/body before relying on it.

A skill's SKILL.md frontmatter supports more than the basics: `paths:` makes it
**conditional** (kept out of the catalog, found via `skill_search` — good for niche
skills that shouldn't bloat every prompt), `always_allow:` lists tool patterns
auto-granted to the session when the skill loads (e.g. `Bash(git *)`), and
`version`/`source_url`/`license` record provenance. Drop extra files in the skill's
folder and reference them from the body with `${SKILL_DIR}/<file>` — they are
advertised on load and read on demand. Use `skill_search <keywords>` to find skills
not shown in the catalog.

`import_skill` brings in an existing **external skill** from `source:"local"` (a
folder path with SKILL.md) or `source:"github"` (a github.com folder URL, e.g.
`https://github.com/owner/repo/tree/main/skills/x`). It maps the skill frontmatter
(allowed-tools→always_allow, paths, version/license/source), copies bundled files,
and reports warnings for unsupported CC features (context:fork, hooks, slash-command
args) — so you can reuse the large CC skill ecosystem without rewriting.

**Artifacts** — `list_artifacts`, `read_artifact` (get content by id — do NOT guess
the file path), `delete_artifact` (delete only agent-created).
`create_artifact` / `update_artifact` are always available — on chat AND on
autonomous (scheduler/spawn/flow) turns (a session-bound sink is installed for
every turn that has a session). For text use
`kind=markdown|code|html|text|svg|mermaid` with `content`. For an image/PDF/binary
FILE you produced on disk (e.g. a screenshot) use `kind=image|video|audio|file` with
`sourcePath` set to the file path — never base64-embed bytes into `content`.
(Screenshots and exported files are also auto-captured from a tool's saved path.)

**Archive (reversible hide)** — agents, skills, artifacts, automations and goals
archive like kanban cards: nothing is deleted, the item leaves the default lists and
the Map ("Harita"), and it can be restored. `list_agents`, `list_artifacts` and
`list_automations` follow the `list_tasks` convention — live items by default,
`archived:true` returns ONLY archived ones. An archived **agent cannot run**: spawning,
delegating, messaging or chatting with it fails with an explicit "is archived" error,
and an automation targeting it is skipped with ledger reason `agent_archived`. It
also cannot be made a NEW target: `create_schedule` / `update_schedule`,
`create_automation` / `update_automation` and `create_task` / `update_task`
(`ownerAgentId`) return the same "is archived" error when the agent id points at an
archived agent. Re-sending a record's already-stored archived agent (e.g. renaming
it) is still accepted; the record just keeps refusing to run until the agent is
restored or retargeted. An
archived **automation never fires** (ledger reason `archived`). An archived **skill** is
never advertised in "# Available Skills", never returned by `skill_search`, and
`use_skill` refuses it with the same explicit error. Archive/restore with the
`set_archived` tool above, in the UI, or over REST (`POST /api/{agents|skills|artifacts|
automations|goals}/{id}/archive` and `/unarchive`); list endpoints take
`?archived=true|false|all`. Tell the user when an archived item blocks the task
instead of working around it.

**Views (projections)** — `get_view` (`{kind, id, level?, lens?, sub?}`) collapses a
large piece of state into a context-cheap compact DSL instead of re-listing it. Four
kinds: `flowrun` (a run tree, optional `sub` for one node), `session`, `board` (the
kanban ledger — `id:"board"`, optional `lens` e.g. `stale`), `workspace`
(`id:"workspace"`). `level` = `tiny|card|full`. Prefer it over repeated
`list_tasks`/`list_sessions` when you only need the shape — a coordinator watching the
board should poll `get_view board` rather than re-listing every turn.

**Logs** — `read_logs` (read the app log ring buffer). Load-on-demand — activate
it like the rest of this suite.

**Processes** — `list_processes` (read-only) lists the native OS processes
TionHarness started for the agents: shell tool calls (foreground + background),
`run_code`/`transform_data` interpreters, claude-cli/codex-cli transports, stdio
MCP servers, hooks and external tool runs, each with its command line, pid,
owner (session + agent), status, exit code and a short output tail. Filter by
`status`/`kind`/`session`. Reach for it before starting yet another long
command — a build that never exited is visible here and nowhere else. There is
no kill tool: stopping a process is a user action in the workspace process panel,
and your OWN background shells are stopped with `shell_manage`.

**Secrets & sessions** — `secret` (one tool, `action: list|get|set|delete`) reads
the encrypted vault and stores/removes a credential, `list_sessions` (enumerate sibling sessions
of EVERY kind — chat + spawn/worker/flow/task/schedule; optional `kind`/`state` filters; a context
block of recent chats is also pushed automatically), `conversation_search` (full-text search across
the workspace's message history — deeper than list_sessions). These are
load-on-demand: `activate_tools` first. (`WebFetch` is NOT here — it is an EAGER
built-in on the native path, always available without activate_tools; on claude-cli
the CLI's own native WebFetch is used.)

**Application settings** — `get_settings`, `update_settings` (read and live-apply
the app-wide settings.json). These change config for the WHOLE application — see
the `tionharness-settings` skill for the full field reference and safety notes.

**Providers** — `list_providers` (READ-ONLY). Lists the configured provider
instances: id, kindId, label, enabled, defaultModel, models and whether the
instance is currently usable. API keys are never returned. Use an `id` from here
as the `provider` value of `create_agent` / `update_agent` to bind an agent to a
specific instance. Provider instances are created/edited/deleted ONLY from the
Settings → Providers screen — there is no tool for that, so don't look for one.

## Working principles

- **Activate narrowly, up front.** One `activate_tools` call for the whole task
  beats activating tool-by-tool.
- **Respect provenance.** Delete/destructive operations are restricted to the
  entities you created — don't try to work around that. (Exception: kanban
  `delete_task` may remove ANY task, including user-created ones.)
- **Prefer reading first.** Use the matching `list_*` / `get_*` tool to see
  current state before you create or edit. Never create an entity (agent, flow,
  skill, schedule, hook, MCP server…) without first checking whether one that
  already does the job exists — extend it instead of duplicating. See
  `tionharness-guide` → "Before you build: discover first".
- **Background work:** `run_subagent` cannot do it — it always blocks until the
  subagent replies, and there is no tool to cancel a running one. For work that
  must outlive the current turn, either split it into several smaller
  self-contained `run_subagent` calls, or become a coordinator
  (`set_coordinator_mode`) and start background workers with `spawn_worker` —
  those are stopped with the coordinator's own `stop_worker`, a different tool.
  Standing automation belongs in a schedule or a flow.
- For a conceptual overview of TionHarness's pieces, load `tionharness-guide`.
