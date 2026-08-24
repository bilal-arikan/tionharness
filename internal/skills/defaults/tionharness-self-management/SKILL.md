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
entities a user would from the UI. They are **gated** (only present when the
workspace has the *Self-management* capability on) and **loaded on demand** — they
do not ship at the start of a turn to keep the prompt lean.

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
> If `activate_tools` reports a self-management name as unknown, the workspace does
> not have self-management enabled (ask the user to turn it on in Settings →
> Capabilities). The vault/session/web tools below (`secret_*`, `list_sessions`,
> `WebFetch`) are load-on-demand independently of self-management — they appear in
> the prompt's load-on-demand block (not hidden) when their own capability (secret
> vault / cross-session context) is on.

## Tool catalog

**Agents** — `list_agents`, `create_agent`, `update_agent`, `delete_agent`.
`create_agent` takes an optional `skills` array (slugs); omit it and the new agent
is seeded with the default TionHarness skill set, unknown slugs are skipped.
Provenance enforced: you can delete only agents you created, never the user's and
never yourself. Deleting an agent cascades: it also removes the agent's sessions,
the schedules bound to it, and the tasks it owns (with their runs) — so delete
deliberately, it is not reversible.

**Agent delegation & messaging** —
- `run_subagent` — launch an isolated worker (a built-in profile or an existing
  agent) and get back only the final result, so the sub-task's tool output never
  floods the current context. Supports sync (wait for reply, default) and async
  (detached background run). Multiple calls in one turn run in parallel.
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

> Built-in run_subagent profiles: `explore` (read-only search), `coder` (write/edit
> code), `reviewer` (read-only review), `config` (mini-agent: quick edits to
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

**Tasks (kanban board)** — `list_tasks`, `create_task`, `update_task`, `move_task`,
`delete_task`. The board is passive (no run tool). Read/create/edit/move/delete
ANY task (including user-created ones) — `delete_task` is irreversible.

**Hooks** — `list_hooks`, `create_hook`, `update_hook`, `delete_hook`. PreToolUse/PostToolUse
external commands that intercept native tool calls (standard hook contract).
Read/create any; delete only ones you created.

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
you're running in, never the last remaining one. A new workspace starts empty
(seeded with the blank template); switch to it in the UI to use it.

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

**Views (projections)** — `get_view` (`{kind, id, level?, lens?, sub?}`) collapses a
large piece of state into a context-cheap compact DSL instead of re-listing it. Four
kinds: `flowrun` (a run tree, optional `sub` for one node), `session`, `board` (the
kanban ledger — `id:"board"`, optional `lens` e.g. `stale`), `workspace`
(`id:"workspace"`). `level` = `tiny|card|full`. Prefer it over repeated
`list_tasks`/`list_sessions` when you only need the shape — a coordinator watching the
board should poll `get_view board` rather than re-listing every turn.

**Logs** — `read_logs` (read the app log ring buffer). Load-on-demand — activate
it like the rest of this suite.

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
- **Background work via `run_subagent`:** to start work that runs without
  blocking the current turn, call `run_subagent` with `wait:"async"` (targets
  an existing agent, not a profile). This internally starts a new detached
  session via the same machinery as the old `spawn_session` primitive.
- For a conceptual overview of TionHarness's pieces, load `tionharness-guide`.
