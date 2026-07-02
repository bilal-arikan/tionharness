package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/db"
	"github.com/bilal-arikan/swarmgo/internal/providers"
)

// Automation self-management tools let an agent create, edit, delete and list
// tag-triggered automations — event-driven rules that spawn a new session
// whenever a session carrying a trigger tag finishes a turn, forming bounded
// self-continuing loops. Provenance is enforced: an agent may only edit/delete
// automations it created, never ones the user made in the UI.

// defaultAutomationMax mirrors the API default so an agent that omits the cap
// still gets a runaway brake.
const defaultAutomationMax = 50

type automationDeps struct {
	db      *db.DB
	actorID string
}

func (d automationDeps) requireCreatedByAgent(ctx context.Context, id string) (db.Automation, error) {
	a, err := d.db.GetAutomation(ctx, id)
	if err != nil {
		return db.Automation{}, fmt.Errorf("no automation with id %q (use list_automations)", id)
	}
	if a.CreatedBy == "" {
		return db.Automation{}, fmt.Errorf("automation %q was created by the user and cannot be edited or deleted by an agent", id)
	}
	return a, nil
}

// CreateAutomationTool creates a tag-triggered automation.
type CreateAutomationTool struct{ d automationDeps }

// NewCreateAutomationTool constructs create_automation.
func NewCreateAutomationTool(database *db.DB, actorID string) CreateAutomationTool {
	return CreateAutomationTool{d: automationDeps{db: database, actorID: actorID}}
}

func (CreateAutomationTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "create_automation",
		Description: "Create a tag-triggered automation: when a session carrying triggerTag finishes a turn, " +
			"its final reply is rendered into promptTemplate ({{result}}, {{title}}, {{tag}}, {{sessionId}}) and a NEW " +
			"session is spawned for targetAgentId. By default the spawned session carries triggerTag too, so it " +
			"re-fires the automation on its own completion — a self-continuing loop bounded by maxIterations. " +
			"To tag an existing session so it participates, use set_session_tags.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"name":{"type":"string","description":"Optional display name"},
				"triggerTag":{"type":"string","description":"The session tag that fires this automation when a tagged session's turn ends"},
				"targetAgentId":{"type":"string","description":"The agent that runs the spawned session (see list_agents)"},
				"promptTemplate":{"type":"string","description":"Prompt for the spawned session; placeholders {{result}} {{title}} {{tag}} {{sessionId}}"},
				"spawnTags":{"type":"array","items":{"type":"string"},"description":"Tags applied to the spawned session (default: [triggerTag] → loop; pass [] to break the loop)"},
				"maxIterations":{"type":"integer","description":"Max total fires before auto-disabling (0 = unlimited; default 50)"},
				"cooldownSec":{"type":"integer","description":"Minimum seconds between fires (default 0)"},
				"enabled":{"type":"boolean","description":"Active immediately (default true)"}
			},
			"required":["triggerTag","targetAgentId","promptTemplate"],
			"additionalProperties":false
		}`),
		Examples: []json.RawMessage{
			json.RawMessage(`{"name":"Research loop","triggerTag":"research-loop","targetAgentId":"AGT3","promptTemplate":"Continue the research. Previous findings:\n{{result}}","maxIterations":20}`),
		},
	}
}

func (t CreateAutomationTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Name           string   `json:"name"`
		TriggerTag     string   `json:"triggerTag"`
		TargetAgentID  string   `json:"targetAgentId"`
		PromptTemplate string   `json:"promptTemplate"`
		SpawnTags      []string `json:"spawnTags"`
		MaxIterations  *int     `json:"maxIterations"`
		CooldownSec    *int     `json:"cooldownSec"`
		Enabled        *bool    `json:"enabled"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErr(err)
	}
	in.TriggerTag = strings.TrimSpace(in.TriggerTag)
	in.TargetAgentID = strings.TrimSpace(in.TargetAgentID)
	if in.TriggerTag == "" || in.TargetAgentID == "" || strings.TrimSpace(in.PromptTemplate) == "" {
		return "", fmt.Errorf("triggerTag, targetAgentId and promptTemplate are all required")
	}
	if _, err := t.d.db.GetAgent(ctx, in.TargetAgentID); err != nil {
		return "", fmt.Errorf("no agent with id %q (use list_agents)", in.TargetAgentID)
	}
	maxIter := defaultAutomationMax
	if in.MaxIterations != nil {
		maxIter = *in.MaxIterations
	}
	cooldown := 0
	if in.CooldownSec != nil {
		cooldown = *in.CooldownSec
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	created, err := t.d.db.CreateAutomation(ctx, db.Automation{
		Name:           strings.TrimSpace(in.Name),
		TriggerTag:     in.TriggerTag,
		TargetAgentID:  in.TargetAgentID,
		PromptTemplate: in.PromptTemplate,
		SpawnTags:      in.SpawnTags,
		MaxIterations:  maxIter,
		CooldownSec:    cooldown,
		Enabled:        enabled,
		CreatedBy:      t.d.actorID,
	})
	if err != nil {
		return "", fmt.Errorf("create automation: %w", err)
	}
	b, _ := json.Marshal(map[string]any{"id": created.ID, "enabled": enabled, "action": "created"})
	return string(b), nil
}

// UpdateAutomationTool edits an agent-created automation.
type UpdateAutomationTool struct{ d automationDeps }

// NewUpdateAutomationTool constructs update_automation.
func NewUpdateAutomationTool(database *db.DB, actorID string) UpdateAutomationTool {
	return UpdateAutomationTool{d: automationDeps{db: database, actorID: actorID}}
}

func (UpdateAutomationTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "update_automation",
		Description: "Edit an agent-created automation (not one made by the user). Pass the id and the fields to change (name, triggerTag, targetAgentId, promptTemplate, spawnTags, maxIterations, cooldownSec, enabled).",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"id":{"type":"string","description":"The automation id (see list_automations)"},
				"name":{"type":"string"},
				"triggerTag":{"type":"string"},
				"targetAgentId":{"type":"string"},
				"promptTemplate":{"type":"string"},
				"spawnTags":{"type":"array","items":{"type":"string"}},
				"maxIterations":{"type":"integer"},
				"cooldownSec":{"type":"integer"},
				"enabled":{"type":"boolean"}
			},
			"required":["id"],
			"additionalProperties":false
		}`),
	}
}

func (t UpdateAutomationTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		ID             string    `json:"id"`
		Name           *string   `json:"name"`
		TriggerTag     *string   `json:"triggerTag"`
		TargetAgentID  *string   `json:"targetAgentId"`
		PromptTemplate *string   `json:"promptTemplate"`
		SpawnTags      *[]string `json:"spawnTags"`
		MaxIterations  *int      `json:"maxIterations"`
		CooldownSec    *int      `json:"cooldownSec"`
		Enabled        *bool     `json:"enabled"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErr(err)
	}
	in.ID = strings.TrimSpace(in.ID)
	if in.ID == "" {
		return "", fmt.Errorf("id is required")
	}
	cur, err := t.d.requireCreatedByAgent(ctx, in.ID)
	if err != nil {
		return "", err
	}
	if in.Name != nil {
		cur.Name = strings.TrimSpace(*in.Name)
	}
	if in.TriggerTag != nil {
		cur.TriggerTag = strings.TrimSpace(*in.TriggerTag)
	}
	if in.TargetAgentID != nil {
		if _, err := t.d.db.GetAgent(ctx, *in.TargetAgentID); err != nil {
			return "", fmt.Errorf("no agent with id %q", *in.TargetAgentID)
		}
		cur.TargetAgentID = *in.TargetAgentID
	}
	if in.PromptTemplate != nil {
		cur.PromptTemplate = *in.PromptTemplate
	}
	if in.SpawnTags != nil {
		cur.SpawnTags = *in.SpawnTags
	}
	if in.MaxIterations != nil {
		cur.MaxIterations = *in.MaxIterations
	}
	if in.CooldownSec != nil {
		cur.CooldownSec = *in.CooldownSec
	}
	if err := t.d.db.UpdateAutomation(ctx, cur); err != nil {
		return "", fmt.Errorf("update automation: %w", err)
	}
	if in.Enabled != nil {
		if err := t.d.db.SetAutomationEnabled(ctx, in.ID, *in.Enabled); err != nil {
			return "", fmt.Errorf("set enabled: %w", err)
		}
	}
	b, _ := json.Marshal(map[string]string{"id": in.ID, "action": "updated"})
	return string(b), nil
}

// DeleteAutomationTool removes an agent-created automation.
type DeleteAutomationTool struct{ d automationDeps }

// NewDeleteAutomationTool constructs delete_automation.
func NewDeleteAutomationTool(database *db.DB, actorID string) DeleteAutomationTool {
	return DeleteAutomationTool{d: automationDeps{db: database, actorID: actorID}}
}

func (DeleteAutomationTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "delete_automation",
		Description: "Delete an agent-created automation (not one made by the user). Pass the automation id.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{"id":{"type":"string","description":"The automation id (see list_automations)"}},
			"required":["id"],
			"additionalProperties":false
		}`),
	}
}

func (t DeleteAutomationTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
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
	if _, err := t.d.requireCreatedByAgent(ctx, in.ID); err != nil {
		return "", err
	}
	if err := t.d.db.DeleteAutomation(ctx, in.ID); err != nil {
		return "", fmt.Errorf("delete automation: %w", err)
	}
	b, _ := json.Marshal(map[string]string{"id": in.ID, "action": "deleted"})
	return string(b), nil
}

// ListAutomationsTool lists the workspace automations with provenance + counters.
type ListAutomationsTool struct{ d automationDeps }

// NewListAutomationsTool constructs list_automations.
func NewListAutomationsTool(database *db.DB, actorID string) ListAutomationsTool {
	return ListAutomationsTool{d: automationDeps{db: database, actorID: actorID}}
}

func (ListAutomationsTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "list_automations",
		Description: "List the tag-triggered automations in this workspace (id, name, triggerTag, targetAgent, enabled, iterationCount/maxIterations, and whether each was created by an agent and is therefore editable/deletable by you).",
		InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
	}
}

func (t ListAutomationsTool) Call(ctx context.Context, _ json.RawMessage) (string, error) {
	autos, err := t.d.db.ListAutomations(ctx)
	if err != nil {
		return "", err
	}
	type row struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		TriggerTag     string `json:"triggerTag"`
		TargetAgentID  string `json:"targetAgentId"`
		Enabled        bool   `json:"enabled"`
		IterationCount int    `json:"iterationCount"`
		MaxIterations  int    `json:"maxIterations"`
		CreatedByAgent bool   `json:"createdByAgent"`
	}
	out := make([]row, 0, len(autos))
	for _, a := range autos {
		out = append(out, row{
			ID:             a.ID,
			Name:           a.Name,
			TriggerTag:     a.TriggerTag,
			TargetAgentID:  a.TargetAgentID,
			Enabled:        a.Enabled,
			IterationCount: a.IterationCount,
			MaxIterations:  a.MaxIterations,
			CreatedByAgent: a.CreatedBy != "",
		})
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}
