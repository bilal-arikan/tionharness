// Package market implements an in-app marketplace: a lightweight, file-based
// registry of shareable "packs". A pack bundles one of the shareable entity
// kinds — skill, agent, provider, flow, workspace, mcp, hook — into a single
// portable JSON envelope (a SwarmPack) that can be browsed, installed into a
// workspace, and produced (published) from an existing entity.
//
// The design mirrors internal/skills: a multi-tier (bundled → global →
// workspace) store that scans only the manifest of each *.swarmpack.json on
// load and reads the (heavier) payload lazily on demand, so the catalog stays
// cheap. A fourth tier, remote, is layered on top from configured registry
// indexes (see remote.go) and downloads its payload at install time.
package market

import "io/fs"

// SchemaV1 is the current pack envelope schema tag.
const SchemaV1 = "swarmpack/v1"

// Pack kinds — the shareable entity types. The first four are the original
// SwarmPack v1 kinds; workspace/mcp were added so the market can share
// workspace templates and MCP tool servers.
const (
	KindSkill     = "skill"
	KindAgent     = "agent"
	KindProvider  = "provider"
	KindFlow      = "flow"
	KindWorkspace = "workspace"
	KindMCP       = "mcp"
	// KindHook is a lifecycle/tool hook imported from a foreign plugin (Claude
	// Code plugin.json / hooks.json). Its scripts ride in Pack.Files and the
	// installer materialises them + rewrites ${CLAUDE_PLUGIN_ROOT}.
	KindHook = "hook"
)

// Source identifies which tier a pack was resolved from. Local tiers override
// each other on id collision (global > bundled) and both override remote.
type Source string

const (
	// SourceBundled is the set of packs shipped embedded in the binary
	// (workspace templates); lowest priority.
	SourceBundled Source = "bundled"
	// SourceGlobal is TionSwarm's data-dir market dir (<DataDir>/market) — the
	// only writable local tier (publish/import land here).
	SourceGlobal Source = "global"
	// SourceRemote is a pack resolved from a remote registry index (downloaded
	// on install). It carries RegistryName for display.
	SourceRemote Source = "remote"
)

// Pack is one marketplace entry: a manifest envelope plus a kind-specific
// payload. The manifest fields are cheap and always loaded; Payload is read from
// disk only on detail/install (see Store.Get).
type Pack struct {
	Schema      string   `json:"schema"`
	ID          string   `json:"id"`
	Kind        string   `json:"kind"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Version     string   `json:"version,omitempty"`
	Author      string   `json:"author,omitempty"`
	Icon        string   `json:"icon,omitempty"`
	Color       string   `json:"color,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	CreatedAt   int64    `json:"createdAt,omitempty"`

	// Payload is the kind-specific body. It is an opaque JSON object at the
	// envelope level; install/publish decode it into the typed payloads below.
	Payload Payload `json:"payload,omitempty"`

	// Files are bundled resource files shipped with the pack, keyed by path relative
	// to the entity's folder (e.g. "references/guide.md"). Consumed by the skill
	// installer to recreate nested resources (progressive-disclosure docs, templates,
	// scripts). JSON base64-encodes the []byte values automatically.
	Files map[string][]byte `json:"files,omitempty"`

	// Source is the tier this pack was resolved from (not persisted in files).
	Source Source `json:"source,omitempty"`
	// Path is the absolute path of the backing .swarmpack.json (not serialised).
	Path string `json:"-"`

	// RegistryName is the display name of the remote registry a remote pack came
	// from (empty for local packs). Set when Source == SourceRemote.
	RegistryName string `json:"registryName,omitempty"`
	// SourceRef, when set, marks this pack as an INGEST source (a GitHub repo/tree)
	// rather than a downloadable payload: install runs the ingest pipeline instead of
	// fetching a .swarmpack.json. Set for catalog entries from directory-site bridges.
	SourceRef *SourceRef `json:"sourceRef,omitempty"`

	// InstalledVersion is decorated by the API from the per-workspace install
	// ledger: the version recorded the last time this pack id was installed here.
	// Empty = never installed via the market. Compare with Version to detect an
	// available update.
	InstalledVersion string `json:"installedVersion,omitempty"`

	// remoteURL/remoteSHA are kept only in-memory for remote packs: the payload
	// download URL and its optional sha256. Not persisted, not serialised.
	remoteURL string `json:"-"`
	remoteSHA string `json:"-"`

	// fsys, when non-nil, is the embedded filesystem a bundled pack was scanned
	// from; Get reads its payload via fs.ReadFile(fsys, Path) instead of the OS.
	// Unexported → never serialised.
	fsys fs.FS
}

// Payload is the kind-specific body of a pack. Only the field matching Kind is
// populated; the others stay nil. Kept as one struct (rather than json.RawMessage)
// so the catalog endpoint can omit it trivially and the typed installers read it
// directly.
type Payload struct {
	Skill     *SkillPayload     `json:"skill,omitempty"`
	Agent     *AgentPayload     `json:"agent,omitempty"`
	Provider  *ProviderPayload  `json:"provider,omitempty"`
	Flow      *FlowPayload      `json:"flow,omitempty"`
	Workspace *WorkspacePayload `json:"workspace,omitempty"`
	MCP       *MCPPayload       `json:"mcp,omitempty"`
	Hook      *HookPayload      `json:"hook,omitempty"`
}

// HookPayload is a single lifecycle/tool hook imported from a foreign plugin.
// Command may contain the ${CLAUDE_PLUGIN_ROOT} placeholder; the installer
// rewrites it to the directory where the pack's bundled scripts (Pack.Files) are
// materialised. Matcher/Event follow TionSwarm's db.Hook semantics.
type HookPayload struct {
	Event      string `json:"event"`             // PreToolUse | PostToolUse | UserPromptSubmit | SessionStart | Stop | SubagentStop | PreCompact | Notification | SessionEnd
	Matcher    string `json:"matcher,omitempty"` // tool-name glob (tool events) or source/trigger selector (some lifecycle events)
	Command    string `json:"command"`           // shell/exec command; ${CLAUDE_PLUGIN_ROOT} rewritten at install
	TimeoutSec int    `json:"timeoutSec,omitempty"`
}

// SkillPayload carries a skill as its portable SKILL.md text (frontmatter +
// body) plus the slug the install should write it under.
type SkillPayload struct {
	Slug string `json:"slug"`
	Body string `json:"body"`
}

// AgentPayload is a sanitised agent config: no IDs, no provenance, no secrets.
// Skills are referenced by slug and resolved (or dropped) at install time.
type AgentPayload struct {
	Name           string   `json:"name"`
	Soul           string   `json:"soul,omitempty"`
	Identity       string   `json:"identity,omitempty"`
	Provider       string   `json:"provider,omitempty"`
	Model          string   `json:"model,omitempty"`
	ThinkingLevel  string   `json:"thinkingLevel,omitempty"`
	PermissionMode string   `json:"permissionMode,omitempty"`
	Avatar         string   `json:"avatar,omitempty"`
	Color          string   `json:"color,omitempty"`
	MCPEnabled     bool     `json:"mcpEnabled,omitempty"`
	AllowedTools   string   `json:"allowedTools,omitempty"`
	Skills         []string `json:"skills,omitempty"`
	// Coordinator defaults for the sessions this agent opens (see
	// db.Agent.CoordinatorMode). Kept in sync with WorkspaceTemplateAgent so a
	// coordinator survives being shared either as a standalone agent pack or as
	// part of a workspace template.
	CoordinatorMode     bool   `json:"coordinatorMode,omitempty"`
	CoordinatorWorkflow string `json:"coordinatorWorkflow,omitempty"`
}

// ProviderPayload is a custom provider config WITHOUT its API key. The key is
// supplied by the installing user (never travels in a pack).
type ProviderPayload struct {
	Label        string `json:"label"`
	Kind         string `json:"kind"` // "openai" | "anthropic"
	BaseURL      string `json:"baseUrl"`
	DefaultModel string `json:"defaultModel,omitempty"`
	Models       string `json:"models,omitempty"`
	// Reasoning declares the endpoint accepts a reasoning-effort control: for
	// "openai" kind a `reasoning_effort` field is sent (mapped from the agent's
	// ThinkingLevel); "anthropic" kind always supports thinking natively. False =
	// the field is omitted (safe default — many OpenAI-compatible servers 400 on
	// an unknown param for non-reasoning models).
	Reasoning bool `json:"reasoning,omitempty"`
	// PromptCache documents prompt-cache behaviour for the budget/usage UI and
	// drives cache_control breakpoint injection: "native" (forwards Anthropic-style
	// cache_control — e.g. OpenRouter, Anthropic endpoints), "auto" (server caches
	// implicitly, no client action), "none" (no prompt caching), "" (unknown).
	PromptCache string `json:"promptCache,omitempty"`
}

// FlowPayload is an agent-agnostic flow draft. Graph node agentId slots are
// blanked at publish time and re-assigned by the user after install.
type FlowPayload struct {
	Name  string `json:"name"`
	Graph string `json:"graph"` // orchestration.Graph JSON
}

// BoardColumn mirrors db.BoardColumnDef without importing the db package, so the
// market envelope stays dependency-free. Used by a workspace template's optional
// kanban layout. The API layer maps it to db.BoardColumnDef.
type BoardColumn struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Color string `json:"color,omitempty"`
}

// TemplateAgentKeyPrefix marks an agent reference inside a template flow graph:
// an agent node's agentId is set to "tmpl:<key>" and substituted for the real
// agent id at seed time (the graph itself stays agent-agnostic and portable).
const TemplateAgentKeyPrefix = "tmpl:"

// WorkspaceTemplateAgent is one seed agent in a workspace template — the full
// agent config (mirrors AgentPayload) so a template can ship a richly-configured
// team, not just name+soul. Key is a local reference used to wire flow nodes and
// schedules to the agent's real ID after creation. Empty Provider/Model fall back
// to the workspace/app default at seed time.
type WorkspaceTemplateAgent struct {
	Key            string   `json:"key"`
	Name           string   `json:"name"`
	Soul           string   `json:"soul,omitempty"`
	Identity       string   `json:"identity,omitempty"`
	Provider       string   `json:"provider,omitempty"`
	Model          string   `json:"model,omitempty"`
	ThinkingLevel  string   `json:"thinkingLevel,omitempty"`
	PermissionMode string   `json:"permissionMode,omitempty"`
	Avatar         string   `json:"avatar,omitempty"`
	Color          string   `json:"color,omitempty"`
	MCPEnabled     bool     `json:"mcpEnabled,omitempty"`
	AllowedTools   string   `json:"allowedTools,omitempty"`  // legacy allowlist (JSON array)
	BlockedTools   string   `json:"blockedTools,omitempty"`  // legacy per-agent denylist (JSON array); folded into ToolOverrides on load
	ToolOverrides  string   `json:"toolOverrides,omitempty"` // per-agent tool override map (JSON object: name/pattern → tier)
	Skills         []string `json:"skills,omitempty"`        // skill slugs to assign (resolved against the seeded skills)
	// CoordinatorMode seeds the agent as a coordinator BY DEFAULT, so every session
	// it opens arrives with the coordination tools — this is what lets a template
	// ship a team that orchestrates out of the box (a PM/CTO pair) instead of one
	// the user must toggle per thread. CoordinatorWorkflow optionally pins a
	// coordinator recipe slug (a bundled kind=coordinator-workflow skill).
	CoordinatorMode     bool   `json:"coordinatorMode,omitempty"`
	CoordinatorWorkflow string `json:"coordinatorWorkflow,omitempty"`
}

// WorkspaceTemplateSkill is a skill bundled with a template: its portable
// SKILL.md text (frontmatter + body) under a slug, plus optional nested resource
// files. Seeding writes these into the workspace skills dir BEFORE agents are
// created so an agent's Skills[] references resolve.
type WorkspaceTemplateSkill struct {
	Slug  string            `json:"slug"`
	Body  string            `json:"body"`
	Files map[string][]byte `json:"files,omitempty"`
}

// WorkspaceTemplateStep is one node of a LINEAR seed flow. AgentKey points at a
// WorkspaceTemplateAgent.Key; Prompt is an orchestration template ({{input}}, {{last}}).
type WorkspaceTemplateStep struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	AgentKey string `json:"agentKey"`
	Prompt   string `json:"prompt"`
}

// WorkspaceTemplateFlow is one seed flow. It is either linear (Steps) or a full
// orchestration graph (Graph) supporting branch/parallel/delay/transform. When
// Graph is set it takes precedence; agent nodes reference agents by
// "tmpl:<key>" in their agentId (substituted at seed time).
type WorkspaceTemplateFlow struct {
	Name  string                  `json:"name"`
	Steps []WorkspaceTemplateStep `json:"steps,omitempty"`
	Graph string                  `json:"graph,omitempty"` // orchestration.Graph JSON; agentId = "tmpl:<key>"
}

// WorkspaceTemplateSchedule is a starter cron schedule. It is always seeded
// DISABLED so it never fires until the user opts in via the Schedules screen.
type WorkspaceTemplateSchedule struct {
	Name     string `json:"name,omitempty"`
	AgentKey string `json:"agentKey"`
	CronExpr string `json:"cronExpr"`
	Prompt   string `json:"prompt"`
}

// WorkspaceTemplateAutomation is a starter automation rule. Like a template flow
// it references its agent by AgentKey (a WorkspaceTemplateAgent.Key) and its flow
// by FlowName rather than by id, so the rule is portable; both are resolved
// against the freshly seeded team at install time and a rule whose reference does
// not resolve is skipped rather than seeded broken.
//
// It is always seeded DISABLED, exactly like WorkspaceTemplateSchedule and the
// built-in board automations: installing a template must never silently start
// spending money on every card move. The value it carries is the WIRING — right
// trigger, right column, right agent, right prompt — so turning the behaviour on
// is one toggle instead of a form.
//
// Mirrors the portable subset of db.Automation. Runtime bookkeeping (iteration
// count, last fired…) and ExpiresAt (an absolute timestamp, meaningless once
// shared) are deliberately absent.
type WorkspaceTemplateAutomation struct {
	Name string `json:"name"`
	// TriggerKind: "tag" | "board" | "token" | "counter" ("" = tag).
	TriggerKind string `json:"triggerKind,omitempty"`
	TriggerTag  string `json:"triggerTag,omitempty"`

	// Board trigger filters (TriggerKind == "board").
	BoardOp        string `json:"boardOp,omitempty"`
	BoardFromState string `json:"boardFromState,omitempty"`
	BoardToState   string `json:"boardToState,omitempty"`
	BoardPriority  int    `json:"boardPriority,omitempty"`
	BoardExclusive bool   `json:"boardExclusive,omitempty"`
	BoardAction    string `json:"boardAction,omitempty"` // "spawn" | "archive"

	// Token trigger (TriggerKind == "token").
	TokenScope     string `json:"tokenScope,omitempty"`
	TokenThreshold int    `json:"tokenThreshold,omitempty"`

	// Counter trigger (TriggerKind == "counter").
	CounterMetric   string `json:"counterMetric,omitempty"`
	CounterScope    string `json:"counterScope,omitempty"`
	CounterInterval int    `json:"counterInterval,omitempty"`

	// Target: an agent (by template key) or a flow (by name). A flow-backed rule
	// leaves AgentKey empty. An archive-action board rule needs neither.
	AgentKey string `json:"agentKey,omitempty"`
	FlowName string `json:"flowName,omitempty"`

	SessionMode    string   `json:"sessionMode,omitempty"` // "spawn" | "continue"
	PromptTemplate string   `json:"promptTemplate,omitempty"`
	SpawnTags      []string `json:"spawnTags,omitempty"`
	MaxIterations  int      `json:"maxIterations,omitempty"` // 0 → seeded at the hard cap
	CooldownSec    int      `json:"cooldownSec,omitempty"`
}

// WorkspacePayload is a workspace template: visual identity + instructions, an
// optional kanban layout, and an optional starter ecosystem — bundled skills,
// a richly-configured agent team, one or more flows (linear or non-linear)
// wiring them, and disabled starter schedules. Install creates a brand-new
// workspace from it and seeds everything; the workspace-create picker reuses the
// same payload to seed a user-named workspace.
type WorkspacePayload struct {
	Name         string        `json:"name"`
	Icon         string        `json:"icon,omitempty"`
	Color        string        `json:"color,omitempty"`
	Instructions string        `json:"instructions,omitempty"`
	Columns      []BoardColumn `json:"columns,omitempty"`

	// Editable config files (all optional). Prompts holds only NON-DEFAULT runtime
	// prompt overrides (key → content, e.g. summary/title/compact); a key at its
	// compiled-in default is omitted. Readme is the free-form config/README.md.
	// Both are seeded as files under <workspace>/config/ on install.
	Prompts map[string]string `json:"prompts,omitempty"`
	Readme  string            `json:"readme,omitempty"`

	// Starter ecosystem (all optional). Seed order: skills → agents → flows →
	// schedules → automations, so agent skill assignments, flow agent-key wiring
	// and an automation's agent/flow references all resolve against things that
	// already exist.
	Skills      []WorkspaceTemplateSkill      `json:"skills,omitempty"`
	Agents      []WorkspaceTemplateAgent      `json:"agents,omitempty"`
	Flows       []WorkspaceTemplateFlow       `json:"flows,omitempty"`
	Schedules   []WorkspaceTemplateSchedule   `json:"schedules,omitempty"`
	Automations []WorkspaceTemplateAutomation `json:"automations,omitempty"`
}

// MCPPayload is a Model Context Protocol server config. Secrets in EnvConfig and
// HeadersConfig are the publisher's responsibility to omit; install adds the
// server to the workspace.
type MCPPayload struct {
	Name string `json:"name"`
	// Description is the one-liner shown in the load-on-demand tool catalog's
	// per-server summary.
	Description string `json:"description,omitempty"`
	Transport   string `json:"transport,omitempty"` // stdio | http
	Command     string `json:"command,omitempty"`   // stdio executable
	Args        string `json:"args,omitempty"`      // JSON array of args
	URL         string `json:"url,omitempty"`       // http endpoint
	EnvConfig   string `json:"envConfig,omitempty"` // JSON object of env vars (stdio)
	// HeadersConfig is a JSON object of request headers (http transport) — the
	// only way an authenticated HTTP MCP server can be shipped in a pack.
	HeadersConfig string `json:"headersConfig,omitempty"`
	// Scope is the connection scope: "shared" (one workspace-wide connection,
	// default) or "scoped" (a live connection per session+agent). Empty = shared.
	Scope string `json:"scope,omitempty"`
}
