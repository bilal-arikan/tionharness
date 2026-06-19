package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/providers"
)

// Agent self-management tools let an agent create, edit, delete and list the
// OTHER agents in its workspace. Provenance is enforced: every agent created
// this way is stamped with CreatedBy = the creating agent's ID, and only
// agent-created agents (CreatedBy != "") may be edited or deleted — an agent can
// never touch an agent the user made in the UI.

// agentDeps carries what the agent-management tools need: the workspace DB, the
// acting agent's ID (the provenance stamp), and callbacks to start/stop a
// worker so a newly created/removed agent's heartbeat takes effect immediately.
type agentDeps struct {
	db          *db.DB
	actorID     string
	startWorker func(agentID string, intervalSec int) error
	stopWorker  func(agentID string)
	// reloadSchedules refreshes the live cron registry after DeleteAgent drops
	// the agent's schedules. Optional; nil in contexts without a scheduler.
	reloadSchedules func(context.Context) error
}

// requireAgentCreatedByAgent loads an agent and verifies it was created by an
// agent (not the user), returning a friendly error otherwise.
func (d agentDeps) requireAgentCreatedByAgent(ctx context.Context, id string) (db.Agent, error) {
	a, err := d.db.GetAgent(ctx, id)
	if err != nil {
		return db.Agent{}, fmt.Errorf("no agent with id %q (use list_agents)", id)
	}
	if a.CreatedBy == "" {
		return db.Agent{}, fmt.Errorf("agent %q was created by the user and cannot be edited or deleted by an agent", a.Name)
	}
	return a, nil
}

// CreateAgentTool creates a new agent in the workspace.
type CreateAgentTool struct{ d agentDeps }

// NewCreateAgentTool constructs create_agent.
func NewCreateAgentTool(database *db.DB, actorID string, startWorker func(string, int) error) CreateAgentTool {
	return CreateAgentTool{d: agentDeps{db: database, actorID: actorID, startWorker: startWorker}}
}

func (CreateAgentTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "create_agent",
		Description: "Create a new AI agent in this workspace. Provide a name and optionally a soul (personality/system prompt), identity, provider and model. The new agent is tagged as created by you, so you can later edit or delete it. Returns the new agent's id.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"name":{"type":"string","description":"Display name for the agent"},
				"soul":{"type":"string","description":"Personality / system prompt that defines how the agent behaves"},
				"identity":{"type":"string","description":"Short identity/role description"},
				"provider":{"type":"string","description":"LLM provider id (e.g. claude-cli, anthropic, minimax). Defaults to the workspace default if omitted."},
				"model":{"type":"string","description":"Model id for the chosen provider"},
				"avatar":{"type":"string","description":"Optional emoji shown in the roster avatar"},
				"color":{"type":"string","description":"Optional hex accent color, e.g. #7c3aed"},
				"heartbeatEnabled":{"type":"boolean","description":"Whether the agent wakes autonomously on a timer"},
				"heartbeatIntervalSec":{"type":"integer","description":"Seconds between autonomous wakes (when heartbeatEnabled)"},
				"heartbeatPrompt":{"type":"string","description":"Prompt delivered on each autonomous wake"}
			},
			"required":["name"],
			"additionalProperties":false
		}`),
		Examples: []json.RawMessage{
			// anthropic provider needs an explicit model id.
			json.RawMessage(`{"name":"Reviewer","provider":"anthropic","model":"claude-sonnet-4-6","soul":"You are a meticulous code reviewer; be terse."}`),
			// claude-cli provider is keyless and needs no model id.
			json.RawMessage(`{"name":"Helper","provider":"claude-cli"}`),
			// Autonomous agent: heartbeat fields are set together.
			json.RawMessage(`{"name":"Watcher","provider":"anthropic","model":"claude-sonnet-4-6","heartbeatEnabled":true,"heartbeatIntervalSec":3600,"heartbeatPrompt":"Check for new issues and triage them."}`),
		},
	}
}

func (t CreateAgentTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Name                 string `json:"name"`
		Soul                 string `json:"soul"`
		Identity             string `json:"identity"`
		Provider             string `json:"provider"`
		Model                string `json:"model"`
		Avatar               string `json:"avatar"`
		Color                string `json:"color"`
		HeartbeatEnabled     bool   `json:"heartbeatEnabled"`
		HeartbeatIntervalSec int    `json:"heartbeatIntervalSec"`
		HeartbeatPrompt      string `json:"heartbeatPrompt"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return "", fmt.Errorf("name is required")
	}
	if in.HeartbeatEnabled && in.HeartbeatIntervalSec <= 0 {
		in.HeartbeatIntervalSec = 3600 // sane default: hourly
	}
	created, err := t.d.db.CreateAgent(ctx, db.Agent{
		Name:                 in.Name,
		Soul:                 in.Soul,
		Identity:             in.Identity,
		Provider:             in.Provider,
		Model:                in.Model,
		Avatar:               in.Avatar,
		Color:                in.Color,
		HeartbeatEnabled:     in.HeartbeatEnabled,
		HeartbeatIntervalSec: in.HeartbeatIntervalSec,
		HeartbeatPrompt:      in.HeartbeatPrompt,
		MCPEnabled:           true,
		CreatedBy:            t.d.actorID,
	})
	if err != nil {
		return "", fmt.Errorf("create agent: %w", err)
	}
	if created.HeartbeatEnabled && t.d.startWorker != nil {
		if err := t.d.startWorker(created.ID, created.HeartbeatIntervalSec); err != nil {
			return "", fmt.Errorf("agent created (%s) but failed to start its heartbeat: %w", created.ID, err)
		}
	}
	b, _ := json.Marshal(map[string]string{"id": created.ID, "name": created.Name, "action": "created"})
	return string(b), nil
}

// UpdateAgentTool edits an agent-created agent's profile.
type UpdateAgentTool struct{ d agentDeps }

// NewUpdateAgentTool constructs update_agent.
func NewUpdateAgentTool(database *db.DB, actorID string) UpdateAgentTool {
	return UpdateAgentTool{d: agentDeps{db: database, actorID: actorID}}
}

func (UpdateAgentTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "update_agent",
		Description: "Edit an existing agent that was created by an agent (not by the user). Pass the agent id and only the fields you want to change. Returns the updated agent id.",
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
				"color":{"type":"string"}
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
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	in.ID = strings.TrimSpace(in.ID)
	if in.ID == "" {
		return "", fmt.Errorf("id is required")
	}
	if _, err := t.d.requireAgentCreatedByAgent(ctx, in.ID); err != nil {
		return "", err
	}
	patch := db.AgentProfilePatch{
		Name:     in.Name,
		Soul:     in.Soul,
		Identity: in.Identity,
		Provider: in.Provider,
		Model:    in.Model,
		Avatar:   in.Avatar,
		Color:    in.Color,
	}
	updated, err := t.d.db.UpdateAgent(ctx, in.ID, patch)
	if err != nil {
		return "", fmt.Errorf("update agent: %w", err)
	}
	b, _ := json.Marshal(map[string]string{"id": updated.ID, "name": updated.Name, "action": "updated"})
	return string(b), nil
}

// DeleteAgentTool removes an agent-created agent.
type DeleteAgentTool struct{ d agentDeps }

// NewDeleteAgentTool constructs delete_agent. reloadSchedules (optional) is run
// after deletion so the agent's now-removed schedules also leave the live cron
// registry.
func NewDeleteAgentTool(database *db.DB, actorID string, stopWorker func(string), reloadSchedules func(context.Context) error) DeleteAgentTool {
	return DeleteAgentTool{d: agentDeps{db: database, actorID: actorID, stopWorker: stopWorker, reloadSchedules: reloadSchedules}}
}

func (DeleteAgentTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "delete_agent",
		Description: "Delete an agent that was created by an agent (not by the user). This also removes the agent's sessions. Pass the agent id. You cannot delete yourself.",
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
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	in.ID = strings.TrimSpace(in.ID)
	if in.ID == "" {
		return "", fmt.Errorf("id is required")
	}
	if in.ID == t.d.actorID {
		return "", fmt.Errorf("an agent cannot delete itself")
	}
	if _, err := t.d.requireAgentCreatedByAgent(ctx, in.ID); err != nil {
		return "", err
	}
	if t.d.stopWorker != nil {
		t.d.stopWorker(in.ID)
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
		Name:        "list_agents",
		Description: "List the agents in this workspace (id, name, provider/model, and whether each was created by an agent and is therefore editable/deletable by you).",
		InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
	}
}

func (t ListAgentsTool) Call(ctx context.Context, _ json.RawMessage) (string, error) {
	agents, err := t.d.db.ListAgents(ctx)
	if err != nil {
		return "", err
	}
	type row struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		Provider       string `json:"provider"`
		Model          string `json:"model"`
		CreatedByAgent bool   `json:"createdByAgent"`
		IsSelf         bool   `json:"isSelf"`
	}
	out := make([]row, 0, len(agents))
	for _, a := range agents {
		out = append(out, row{
			ID:             a.ID,
			Name:           a.Name,
			Provider:       a.Provider,
			Model:          a.Model,
			CreatedByAgent: a.CreatedBy != "",
			IsSelf:         a.ID == t.d.actorID,
		})
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}
