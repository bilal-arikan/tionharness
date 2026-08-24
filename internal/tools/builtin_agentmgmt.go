package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// repairMojibake undoes the classic corruption where UTF-8 bytes were mis-decoded
// as Latin-1 before reaching us (e.g. an emoji "🗺️" arriving as "ðºï¸"). Agent
// fields created over the claude-cli tool path occasionally suffer this in the
// CLI/MCP transport. It only acts when every rune fits in the Latin-1 range (so
// genuine multi-byte glyphs are untouched) and the byte reinterpretation is
// itself valid UTF-8 that differs from the input; otherwise the original is
// returned unchanged. This guards real prose: Turkish ş/ğ/ı sit above U+00FF and
// fail the range check, and arbitrary Latin-1 text fails the strict UTF-8
// re-decode.
func repairMojibake(s string) string {
	if s == "" {
		return s
	}
	buf := make([]byte, 0, len(s))
	for _, r := range s {
		if r > 0xFF {
			return s // a genuine high codepoint → not mojibake
		}
		buf = append(buf, byte(r))
	}
	if !utf8.Valid(buf) {
		return s
	}
	out := string(buf)
	if out == s {
		return s // pure ASCII / nothing to repair
	}
	return out
}

// Agent self-management tools let an agent create, edit, delete and list the
// OTHER agents in its workspace. Every agent created this way is stamped with
// CreatedBy = the creating agent's ID for provenance/display, but there is no
// provenance gate: any agent — user- or agent-created — may be edited or deleted.

// agentDeps carries what the agent-management tools need: the workspace DB and
// the acting agent's ID (the provenance stamp).
type agentDeps struct {
	db      *db.DB
	actorID string
	// reloadSchedules refreshes the live cron registry after DeleteAgent drops
	// the agent's schedules. Optional; nil in contexts without a scheduler.
	reloadSchedules func(context.Context) error
	// agentBusy reports whether an agent has a turn or run in flight, so a delete
	// cannot pull the roster entry out from under work that is still writing. The
	// live registries are not reachable from this package, so the runtime injects
	// its AgentBusy. Optional; nil = no guard (the HTTP path keeps its own).
	agentBusy func(context.Context, string) (bool, string)
	// resolveProvider resolves a provider INSTANCE id to the (kind, instanceID)
	// pair to persist (_Docs/71 §2.5, K3) — the tools package cannot import
	// internal/agent's SyncProviderFields directly (internal/agent already
	// imports internal/tools, so the reverse import would cycle), so the
	// runtime injects its bound implementation. nil = 1:1 legacy mirror
	// (kind-id-only, no instance resolution) for contexts without a registry.
	resolveProvider func(instanceID string) (kind, providerInstanceID string, err error)
}

// syncProvider resolves instanceID via the injected resolver, falling back to
// the historical 1:1 kind-mirror when no resolver was wired — including the
// same empty-id → keyless claude-cli default SyncProviderFields applies, so
// callers see identical behaviour whether or not a registry is wired.
func (d agentDeps) syncProvider(instanceID string) (kind, providerInstanceID string, err error) {
	if d.resolveProvider != nil {
		return d.resolveProvider(instanceID)
	}
	if instanceID == "" {
		instanceID = "claude-cli"
	}
	return instanceID, instanceID, nil
}

// requireAgent loads an agent by id, returning a friendly error if it does not
// exist. No provenance gate: user- and agent-created agents are both editable.
func (d agentDeps) requireAgent(ctx context.Context, id string) (db.Agent, error) {
	a, err := d.db.GetAgent(ctx, id)
	if err != nil {
		return db.Agent{}, fmt.Errorf("no agent with id %q (use list_agents)", id)
	}
	return a, nil
}

// CreateAgentTool creates a new agent in the workspace.
type CreateAgentTool struct {
	d agentDeps
	// defaultSkills is the baseline skill-slug set every new agent is seeded with
	// when the caller doesn't provide its own (the shipped TionHarness defaults).
	defaultSkills []string
	// skillExists reports whether a skill slug is known (global/workspace), so
	// caller-provided slugs are validated and unknown ones skipped (no dangling
	// refs). nil accepts any slug.
	skillExists func(slug string) bool
}

// NewCreateAgentTool constructs create_agent. defaultSkills seeds new agents when
// the caller passes none; skillExists validates caller-supplied slugs (nil = allow
// all). resolveProvider resolves a provider instance id to (kind, instanceID)
// (nil = legacy 1:1 mirror, see agentDeps.resolveProvider). All may be nil/empty
// in minimal contexts.
func NewCreateAgentTool(database *db.DB, actorID string, defaultSkills []string, skillExists func(slug string) bool, resolveProvider func(string) (string, string, error)) CreateAgentTool {
	return CreateAgentTool{d: agentDeps{db: database, actorID: actorID, resolveProvider: resolveProvider}, defaultSkills: defaultSkills, skillExists: skillExists}
}

func (CreateAgentTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "create_agent",
		Description: "Create a new AI agent in this workspace. Provide a name and optionally a soul (personality/system prompt), identity, provider, model and skills. The new agent is tagged as created by you, so you can later edit or delete it. If you omit \"skills\", the agent is seeded with the default TionHarness skill set. Returns the new agent's id.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"name":{"type":"string","description":"Display name for the agent"},
				"soul":{"type":"string","description":"Personality / system prompt that defines how the agent behaves"},
				"identity":{"type":"string","description":"Short identity/role description"},
				"provider":{"type":"string","description":"LLM provider id (e.g. claude-cli, anthropic, minimax). If omitted, inherits the creating agent's provider (together with its model), falling back to claude-cli."},
				"model":{"type":"string","description":"Model id for the chosen provider. If both provider and model are omitted, both are inherited from the creating agent."},
				"avatar":{"type":"string","description":"Optional emoji shown in the roster avatar"},
				"color":{"type":"string","description":"Optional hex accent color, e.g. #7c3aed"},
				"skills":{"type":"array","items":{"type":"string"},"description":"Skill slugs to enable for the agent (use_skill). Omit to seed the default TionHarness skill set; unknown slugs are skipped."},
				"coordinatorPrompt":{"type":"string","description":"Orchestration guidance injected ONLY while the agent's session is in coordinator mode (right after the shared coordinator manual). Put delegation direction here instead of in the soul, so it costs nothing when the agent is not coordinating."}
			},
			"required":["name"],
			"additionalProperties":false
		}`),
		Examples: []json.RawMessage{
			// anthropic provider needs an explicit model id.
			json.RawMessage(`{"name":"Reviewer","provider":"anthropic","model":"claude-sonnet-5","soul":"You are a meticulous code reviewer; be terse."}`),
			// claude-cli provider is keyless and needs no model id.
			json.RawMessage(`{"name":"Helper","provider":"claude-cli"}`),
		},
	}
}

func (t CreateAgentTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Name     string   `json:"name"`
		Soul     string   `json:"soul"`
		Identity string   `json:"identity"`
		Provider string   `json:"provider"`
		Model    string   `json:"model"`
		Avatar   string   `json:"avatar"`
		Color    string   `json:"color"`
		Skills   []string `json:"skills"`
		// CoordinatorPrompt is injected only while the agent coordinates.
		CoordinatorPrompt string `json:"coordinatorPrompt"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErr(err)
	}
	// Repair any UTF-8→Latin-1 mojibake introduced by the CLI/MCP transport so
	// emoji avatars and prose are stored correctly (no garbled glyphs in the UI).
	in.Name = repairMojibake(in.Name)
	in.Soul = repairMojibake(in.Soul)
	in.Identity = repairMojibake(in.Identity)
	in.Avatar = repairMojibake(in.Avatar)
	in.CoordinatorPrompt = repairMojibake(in.CoordinatorPrompt)
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return "", fmt.Errorf("name is required")
	}
	// Default the provider so the agent is runnable: an empty provider produced
	// agents (e.g. flow nodes) whose turns relied on implicit fallback and were
	// hard to diagnose. When the caller omits the provider, inherit it — as a
	// matched provider+model pair — from the creating agent: a coordinator running
	// on e.g. deepseek that spawns sub-agents gets them on the same provider/model
	// instead of forcing every new agent back to claude-cli (the app has no
	// abstract per-workspace default; provider/model is agent-based). The model is
	// inherited ONLY when the caller gave neither provider nor model, so an
	// inherited model can never be paired with a mismatched caller-chosen provider.
	// A missing/unknown creator (e.g. empty actor id on flow nodes) simply falls
	// through to claude-cli, the keyless final fallback. Not codex-cli: same
	// reasoning as defaultProviderModel (api/templates.go) — the anonymous-creator
	// fallback should land on the provider most likely to already be usable
	// out of the box, and codex-cli is opt-in like every other provider.
	in.Provider = strings.TrimSpace(in.Provider)
	in.Model = strings.TrimSpace(in.Model)
	// in.Provider is accepted as a provider INSTANCE id (_Docs/71 §5); resolved
	// via the injected resolver so a non-default instance (e.g. "PRV3") works,
	// not just ids that happen to equal their kind. Inheriting from the creator
	// carries ITS instance id too — not just its kind — so a creator bound to a
	// non-default instance propagates correctly instead of silently falling
	// back to the default instance of that kind.
	instanceInput := in.Provider
	if instanceInput == "" {
		if creator, err := t.d.db.GetAgent(ctx, t.d.actorID); err == nil && creator.Provider != "" {
			instanceInput = creator.ProviderInstanceID
			if instanceInput == "" {
				instanceInput = creator.Provider
			}
			if in.Model == "" {
				in.Model = creator.Model
			}
		}
	}
	providerKind, providerInstanceID, err := t.d.syncProvider(instanceInput)
	if err != nil {
		return "", err
	}
	in.Provider = providerKind

	// Resolve the skill set: caller-provided (validated) or, when none given, the
	// default TionHarness set. Unknown caller slugs are dropped and reported.
	skills, skipped := t.resolveSkills(in.Skills)

	created, err := t.d.db.CreateAgent(ctx, db.Agent{
		Name:               in.Name,
		Soul:               in.Soul,
		Identity:           in.Identity,
		Provider:           in.Provider,
		ProviderInstanceID: providerInstanceID,
		Model:              in.Model,
		Avatar:             in.Avatar,
		Color:              in.Color,
		Skills:             skills,
		MCPEnabled:         true,
		CreatedBy:          t.d.actorID,

		CoordinatorPrompt: in.CoordinatorPrompt,
	})
	if err != nil {
		return "", fmt.Errorf("create agent: %w", err)
	}
	out := map[string]any{"id": created.ID, "name": created.Name, "action": "created", "skills": skills}
	if len(skipped) > 0 {
		out["skippedUnknownSkills"] = skipped
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}

// resolveSkills validates caller-provided slugs (dropping+reporting unknown ones)
// and falls back to the default skill set when the caller gave none. Order is
// preserved and duplicates removed.
func (t CreateAgentTool) resolveSkills(provided []string) (skills, skipped []string) {
	seen := map[string]bool{}
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			return
		}
		if t.skillExists != nil && !t.skillExists(s) {
			skipped = append(skipped, s)
			return
		}
		seen[s] = true
		skills = append(skills, s)
	}
	if len(provided) > 0 {
		for _, s := range provided {
			add(s)
		}
		return skills, skipped
	}
	for _, s := range t.defaultSkills {
		add(s)
	}
	return skills, skipped
}

// UpdateAgentTool edits an agent-created agent's profile.
type UpdateAgentTool struct{ d agentDeps }

// NewUpdateAgentTool constructs update_agent. resolveProvider resolves a
// provider instance id to (kind, instanceID) (nil = legacy 1:1 mirror, see
// agentDeps.resolveProvider).
func NewUpdateAgentTool(database *db.DB, actorID string, resolveProvider func(string) (string, string, error)) UpdateAgentTool {
	return UpdateAgentTool{d: agentDeps{db: database, actorID: actorID, resolveProvider: resolveProvider}}
}

func (UpdateAgentTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "update_agent",
		Description: "Edit an existing agent (user- or agent-created). Pass the agent id and only the fields you want to change. Returns the updated agent id.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"id":{"type":"string","description":"The agent id to update (see list_agents)"},
				"name":{"type":"string"},
				"soul":{"type":"string"},
				"identity":{"type":"string"},
				"provider":{"type":"string"},
				"model":{"type":"string"},
				"avatar":{"type":"string"},
				"color":{"type":"string"},
				"coordinatorPrompt":{"type":"string","description":"Orchestration guidance injected ONLY while the agent's session is in coordinator mode (right after the shared coordinator manual). Pass \"\" to clear it."}
			},
			"required":["id"],
			"additionalProperties":false
		}`),
	}
}

func (t UpdateAgentTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		ID       string  `json:"id"`
		Name     *string `json:"name"`
		Soul     *string `json:"soul"`
		Identity *string `json:"identity"`
		Provider *string `json:"provider"`
		Model    *string `json:"model"`
		Avatar   *string `json:"avatar"`
		Color    *string `json:"color"`
		// CoordinatorPrompt: omitted leaves it alone, "" clears it.
		CoordinatorPrompt *string `json:"coordinatorPrompt"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErr(err)
	}
	in.ID = strings.TrimSpace(in.ID)
	if in.ID == "" {
		return "", fmt.Errorf("id is required")
	}
	previous, err := t.d.requireAgent(ctx, in.ID)
	if err != nil {
		return "", err
	}
	// Repair any UTF-8→Latin-1 mojibake from the CLI/MCP transport before saving.
	for _, p := range []*string{in.Name, in.Soul, in.Identity, in.Avatar, in.CoordinatorPrompt} {
		if p != nil {
			*p = repairMojibake(*p)
		}
	}
	patch := db.AgentProfilePatch{
		Name:     in.Name,
		Soul:     in.Soul,
		Identity: in.Identity,
		Model:    in.Model,
		Avatar:   in.Avatar,
		Color:    in.Color,

		CoordinatorPrompt: in.CoordinatorPrompt,
	}
	// in.Provider is accepted as a provider INSTANCE id (_Docs/71 §5); resolved
	// via the injected resolver so a non-default instance (e.g. "PRV3") works,
	// not just ids that happen to equal their kind. Only sync when the request
	// actually touches it (nil = "not in this patch").
	if in.Provider != nil {
		providerKind, providerInstanceID, err := t.d.syncProvider(*in.Provider)
		if err != nil {
			return "", err
		}
		patch.Provider = &providerKind
		patch.ProviderInstanceID = &providerInstanceID
	}
	updated, err := t.d.db.UpdateAgent(ctx, in.ID, patch)
	if err != nil {
		return "", fmt.Errorf("update agent: %w", err)
	}
	if in.Model != nil && *in.Model != previous.Model {
		notifyAgentModelChanged(t.d.db, AgentModelChange{AgentID: updated.ID, AgentName: updated.Name, OldModel: previous.Model, NewModel: *in.Model})
	}
	result := map[string]string{"id": updated.ID, "name": updated.Name, "action": "updated"}
	if in.Model != nil {
		if warning := AgentModelChangeWarning(updated); warning != "" {
			result["warning"] = warning
		}
	}
	b, _ := json.Marshal(result)
	return string(b), nil
}

// DeleteAgentTool removes an agent-created agent.
type DeleteAgentTool struct{ d agentDeps }

// NewDeleteAgentTool constructs delete_agent. reloadSchedules (optional) is run
// after deletion so the agent's now-removed schedules also leave the live cron
// registry. agentBusy (optional) is the same in-flight check the HTTP delete
// endpoint uses — without it an agent could delete a peer mid-turn, which the
// user-facing path already refuses.
func NewDeleteAgentTool(
	database *db.DB,
	actorID string,
	reloadSchedules func(context.Context) error,
	agentBusy func(context.Context, string) (bool, string),
) DeleteAgentTool {
	return DeleteAgentTool{d: agentDeps{
		db:              database,
		actorID:         actorID,
		reloadSchedules: reloadSchedules,
		agentBusy:       agentBusy,
	}}
}

func (DeleteAgentTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "delete_agent",
		Description: "Delete an agent (user- or agent-created). This also removes the agent's sessions. Pass the agent id. You cannot delete yourself.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{"id":{"type":"string","description":"The agent id to delete (see list_agents)"}},
			"required":["id"],
			"additionalProperties":false
		}`),
	}
}

func (t DeleteAgentTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErr(err)
	}
	in.ID = strings.TrimSpace(in.ID)
	if in.ID == "" {
		return "", fmt.Errorf("id is required")
	}
	if in.ID == t.d.actorID {
		return "", fmt.Errorf("an agent cannot delete itself")
	}
	if _, err := t.d.requireAgent(ctx, in.ID); err != nil {
		return "", err
	}
	// Same guard the HTTP delete enforces: a target with a turn or run in flight
	// is not deletable. Skipping it here would let an agent do through a tool
	// exactly what the user is refused in the UI.
	if t.d.agentBusy != nil {
		if busy, where := t.d.agentBusy(ctx, in.ID); busy {
			return "", fmt.Errorf("agent %s is busy (%s) — stop its turn before deleting it", in.ID, where)
		}
	}
	if err := t.d.db.DeleteAgent(ctx, in.ID); err != nil {
		return "", fmt.Errorf("delete agent: %w", err)
	}
	// Drop the deleted agent's schedules from the live cron registry too.
	if t.d.reloadSchedules != nil {
		_ = t.d.reloadSchedules(ctx)
	}
	b, _ := json.Marshal(map[string]string{"id": in.ID, "action": "deleted"})
	return string(b), nil
}

// ListAgentsTool lists the agents in the workspace with their provenance.
type ListAgentsTool struct{ d agentDeps }

// NewListAgentsTool constructs list_agents.
func NewListAgentsTool(database *db.DB, actorID string) ListAgentsTool {
	return ListAgentsTool{d: agentDeps{db: database, actorID: actorID}}
}

func (ListAgentsTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "list_agents",
		Description: "List the agents in this workspace (id, name, state, provider/model, and whether each was " +
			"created by an agent — provenance only; you can edit/delete any of them). Results are PAGINATED: " +
			"pass limit (default 20, max 100) and offset to page; the reply reports total and hasMore, and you " +
			"reach the next page with offset += limit. Filters: provider (case-insensitive substring), model " +
			"(case-insensitive substring), state (enabled = the active roster, the default; disabled = agents " +
			"marked deleted but kept so past conversations still render their author). Sort: updated_desc " +
			"(default), updated_asc, created_desc, created_asc, name_asc, name_desc.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "provider": { "type": "string", "description": "Only agents whose provider contains this substring (case-insensitive)." },
    "model": { "type": "string", "description": "Only agents whose model contains this substring (case-insensitive)." },
    "state": { "type": "string", "enum": ["enabled", "disabled"], "description": "enabled (default) = active roster; disabled = deleted-but-kept agents." },
    "sort": { "type": "string", "enum": ["updated_desc", "updated_asc", "created_desc", "created_asc", "name_asc", "name_desc"], "description": "Result ordering (default updated_desc)." },
    "limit": { "type": "integer", "description": "Max agents per page (default 20, max 100)." },
    "offset": { "type": "integer", "description": "How many matching agents to skip before this page (default 0)." }
  },
  "additionalProperties": false
}`),
	}
}

func (t ListAgentsTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
		State    string `json:"state"`
		Sort     string `json:"sort"`
		Limit    int    `json:"limit"`
		Offset   int    `json:"offset"`
	}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &in); err != nil {
			return "", argErr(err)
		}
	}
	limit, offset := PageArgs(in.Limit, in.Offset)

	// state selects the source roster: enabled = the live roster (deleted agents
	// are excluded by ListAgents); disabled = only the deleted-but-kept rows that
	// history rendering still resolves.
	var agents []db.Agent
	var err error
	switch strings.TrimSpace(in.State) {
	case "", "enabled":
		agents, err = t.d.db.ListAgents(ctx)
	case "disabled":
		all, e := t.d.db.ListAgentsWithDeleted(ctx)
		if e != nil {
			return "", e
		}
		for _, a := range all {
			if a.Deleted {
				agents = append(agents, a)
			}
		}
	default:
		return "", fmt.Errorf("state must be \"enabled\" or \"disabled\", got %q", in.State)
	}
	if err != nil {
		return "", err
	}

	provider := strings.ToLower(strings.TrimSpace(in.Provider))
	model := strings.ToLower(strings.TrimSpace(in.Model))
	matches := make([]db.Agent, 0, len(agents))
	for _, a := range agents {
		if provider != "" && !strings.Contains(strings.ToLower(a.Provider), provider) {
			continue
		}
		if model != "" && !strings.Contains(strings.ToLower(a.Model), model) {
			continue
		}
		matches = append(matches, a)
	}

	field, asc, err := SortOrder(in.Sort)
	if err != nil {
		return "", err
	}
	less, err := SortByField(matches, field, asc,
		func(a db.Agent) int64 { return a.UpdatedAt },
		func(a db.Agent) int64 { return a.CreatedAt },
		func(a db.Agent) string { return a.Name },
		func(a db.Agent) string { return a.ID },
	)
	if err != nil {
		return "", err
	}
	sort.SliceStable(matches, less)

	page, total := SlicePage(matches, offset, limit)
	type row struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		State          string `json:"state"`
		Provider       string `json:"provider"`
		Model          string `json:"model"`
		CreatedByAgent bool   `json:"createdByAgent"`
		IsSelf         bool   `json:"isSelf"`
	}
	out := make([]row, 0, len(page))
	for _, a := range page {
		state := "enabled"
		if a.Deleted {
			state = "disabled"
		}
		out = append(out, row{
			ID:             a.ID,
			Name:           a.Name,
			State:          state,
			Provider:       a.Provider,
			Model:          a.Model,
			CreatedByAgent: a.CreatedBy != "",
			IsSelf:         a.ID == t.d.actorID,
		})
	}
	return pageResult(out, total, offset, limit)
}
