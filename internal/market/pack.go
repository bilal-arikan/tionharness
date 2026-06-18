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

// SchemaV1 is the current pack envelope schema tag.
const SchemaV1 = "swarmpack/v1"

// Pack kinds — the four shareable entity types.
const (
	KindSkill    = "skill"
	KindAgent    = "agent"
	KindProvider = "provider"
	KindFlow     = "flow"
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

	// Source is the tier this pack was resolved from (not persisted in files).
	Source Source `json:"source,omitempty"`
	// Path is the absolute path of the backing .swarmpack.json (not serialised).
	Path string `json:"-"`
}

// Payload is the kind-specific body of a pack. Only the field matching Kind is
// populated; the others stay nil. Kept as one struct (rather than json.RawMessage)
// so the catalog endpoint can omit it trivially and the typed installers read it
// directly.
type Payload struct {
	Skill    *SkillPayload    `json:"skill,omitempty"`
	Agent    *AgentPayload    `json:"agent,omitempty"`
	Provider *ProviderPayload `json:"provider,omitempty"`
	Flow     *FlowPayload     `json:"flow,omitempty"`
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
	Capabilities   string   `json:"capabilities,omitempty"`
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
}

// FlowPayload is an agent-agnostic flow draft. Graph node agentId slots are
// blanked at publish time and re-assigned by the user after install.
type FlowPayload struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Graph       string `json:"graph"` // orchestration.Graph JSON
}
