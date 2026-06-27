// Package market implements an in-app marketplace: a lightweight, file-based
// registry of shareable "packs". A pack bundles one of the four shareable entity
// kinds — skill, agent, provider, flow — into a single portable JSON envelope
// (a SwarmPack) that can be browsed, installed into a workspace, and produced
// (published) from an existing entity.
//
// The design mirrors internal/skills: a multi-tier (bundled → global →
// workspace) store that scans only the manifest of each *.swarmpack.json on
// load and reads the (heavier) payload lazily on demand, so the catalog stays
// cheap.
package market

import "io/fs"

// SchemaV1 is the current pack envelope schema tag.
const SchemaV1 = "swarmpack/v1"

// Pack kinds — the shareable entity types. The first four are the original
// SwarmPack v1 kinds; workspace/memory/mcp were added so the market can share
// workspace templates, seed memories and MCP tool servers.
const (
	KindSkill     = "skill"
	KindAgent     = "agent"
	KindProvider  = "provider"
	KindFlow      = "flow"
	KindWorkspace = "workspace"
	KindMemory    = "memory"
	KindMCP       = "mcp"
)

// Source identifies which tier a pack was resolved from. Higher tiers override
// lower ones on id collision (workspace > global > bundled).
type Source string

const (
	// SourceBundled is the set of packs shipped embedded in the binary.
	SourceBundled Source = "bundled"
	// SourceGlobal is SwarmGo's data-dir market dir (<DataDir>/market).
	SourceGlobal Source = "global"
	// SourceWorkspace is this workspace's market dir (<workspace>/market).
	SourceWorkspace Source = "workspace"
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
	Memory    *MemoryPayload    `json:"memory,omitempty"`
	MCP       *MCPPayload       `json:"mcp,omitempty"`
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
	PlanningMode   string   `json:"planningMode,omitempty"`
	ThinkingLevel  string   `json:"thinkingLevel,omitempty"`
	PermissionMode string   `json:"permissionMode,omitempty"`
	Avatar         string   `json:"avatar,omitempty"`
	Color          string   `json:"color,omitempty"`
	MCPEnabled     bool     `json:"mcpEnabled,omitempty"`
	AllowedTools   string   `json:"allowedTools,omitempty"`
	Skills         []string `json:"skills,omitempty"`
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
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Graph       string `json:"graph"` // orchestration.Graph JSON
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
	PlanningMode   string   `json:"planningMode,omitempty"`
	ThinkingLevel  string   `json:"thinkingLevel,omitempty"`
	PermissionMode string   `json:"permissionMode,omitempty"`
	Avatar         string   `json:"avatar,omitempty"`
	Color          string   `json:"color,omitempty"`
	MCPEnabled     bool     `json:"mcpEnabled,omitempty"`
	AllowedTools   string   `json:"allowedTools,omitempty"` // legacy allowlist (JSON array)
	BlockedTools   string   `json:"blockedTools,omitempty"` // per-agent denylist (JSON array)
	Skills         []string `json:"skills,omitempty"`       // skill slugs to assign (resolved against the seeded skills)
	DailyCallLimit  int     `json:"dailyCallLimit,omitempty"`
	DailyTokenLimit int     `json:"dailyTokenLimit,omitempty"`
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
	Name        string                  `json:"name"`
	Description string                  `json:"description,omitempty"`
	Steps       []WorkspaceTemplateStep `json:"steps,omitempty"`
	Graph       string                  `json:"graph,omitempty"` // orchestration.Graph JSON; agentId = "tmpl:<key>"
}

// WorkspaceTemplateSchedule is a starter cron schedule. It is always seeded
// DISABLED so it never fires until the user opts in via the Schedules screen.
type WorkspaceTemplateSchedule struct {
	AgentKey string `json:"agentKey"`
	CronExpr string `json:"cronExpr"`
	Prompt   string `json:"prompt"`
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

	// Starter ecosystem (all optional). Seed order: skills → agents → flows →
	// schedules, so agent skill assignments and flow agent-key wiring resolve.
	Skills    []WorkspaceTemplateSkill    `json:"skills,omitempty"`
	Agents    []WorkspaceTemplateAgent    `json:"agents,omitempty"`
	Flows     []WorkspaceTemplateFlow     `json:"flows,omitempty"`
	Schedules []WorkspaceTemplateSchedule `json:"schedules,omitempty"`
}

// MCPPayload is a Model Context Protocol server config. Secrets in EnvConfig are
// the publisher's responsibility to omit; install adds the server to the workspace.
type MCPPayload struct {
	Name      string `json:"name"`
	Transport string `json:"transport,omitempty"` // stdio | sse | http
	Command   string `json:"command,omitempty"`   // stdio executable
	Args      string `json:"args,omitempty"`      // JSON array of args
	URL       string `json:"url,omitempty"`       // sse/http endpoint
	EnvConfig string `json:"envConfig,omitempty"` // JSON object of env vars
}

// MemoryEntry is one seed memory in a memory pack.
type MemoryEntry struct {
	Content string `json:"content"`
	Kind    string `json:"kind,omitempty"` // default "document"
}

// MemoryPayload seeds a set of memories into the installing workspace's first agent.
type MemoryPayload struct {
	Entries []MemoryEntry `json:"entries"`
}
