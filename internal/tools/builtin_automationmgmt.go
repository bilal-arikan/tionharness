package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
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
		Description: "Create an event-driven automation. Two trigger kinds: (a) triggerKind='tag' (default) — when a session " +
			"carrying triggerTag finishes a turn, its final reply is rendered into promptTemplate ({{result}}, {{title}}, " +
			"{{tag}}, {{sessionId}}) and the target runs; (b) triggerKind='board' — when a kanban card changes (created/moved/" +
			"updated/deleted), the target runs with the card context ({{taskId}}, {{title}}, {{op}}, {{from}}, {{to}}, " +
			"{{toLabel}}). The target is EITHER an agent (targetAgentId → a NEW session is spawned) OR an orchestration flow " +
			"(flowId → the rendered prompt is run as the flow input). For a tag automation the spawned session carries " +
			"triggerTag by default (a self-continuing loop bounded by maxIterations); board automations do not self-loop.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"name":{"type":"string","description":"Optional display name"},
				"triggerKind":{"type":"string","enum":["tag","board"],"description":"What fires the automation: 'tag' (default; session tag) or 'board' (kanban card change)"},
				"triggerTag":{"type":"string","description":"[tag kind] The session tag that fires this automation when a tagged session's turn ends"},
				"boardOp":{"type":"string","enum":["any","move","create","update","delete"],"description":"[board kind] Which card change fires it (default 'move')"},
				"boardFromState":{"type":"string","description":"[board kind] Only fire when a card LEAVES this column (empty = any source)"},
				"boardToState":{"type":"string","description":"[board kind] Only fire when a card ENTERS this column (empty = any target)"},
				"targetAgentId":{"type":"string","description":"The agent that runs the spawned session (see list_agents). Omit when flowId is set."},
				"flowId":{"type":"string","description":"Run this orchestration flow with the rendered prompt as its input instead of spawning an agent session (see list_flows)."},
				"promptTemplate":{"type":"string","description":"Prompt for the spawned session (or flow input). Tag placeholders: {{result}}, {{title}}, {{tag}}, {{sessionId}}, {{prevPrompt}}, {{agent}}. Board placeholders: {{taskId}}, {{title}}, {{op}}, {{from}}, {{to}}, {{fromLabel}}, {{toLabel}}, {{board}}. Common: {{iteration}}, {{maxIterations}}, {{automation}}, {{date}}, {{time}}, {{datetime}}"},
				"spawnTags":{"type":"array","items":{"type":"string"},"description":"Tags applied to the spawned session (tag kind default: [triggerTag] → loop; pass [] to break the loop). Ignored for flow-backed and board automations."},
				"maxIterations":{"type":"integer","description":"Max total fires before auto-disabling (0 = unlimited; default 50)"},
				"cooldownSec":{"type":"integer","description":"Minimum seconds between fires (default 0)"},
				"expiresAt":{"type":"integer","description":"Optional end date (unix seconds); after it the automation auto-disables. 0 = no end date"},
				"enabled":{"type":"boolean","description":"Active immediately (default true)"}
			},
			"required":["promptTemplate"],
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
		TriggerKind    string   `json:"triggerKind"`
		TriggerTag     string   `json:"triggerTag"`
		BoardOp        string   `json:"boardOp"`
		BoardFromState string   `json:"boardFromState"`
		BoardToState   string   `json:"boardToState"`
		TargetAgentID  string   `json:"targetAgentId"`
		FlowID         string   `json:"flowId"`
		PromptTemplate string   `json:"promptTemplate"`
		SpawnTags      []string `json:"spawnTags"`
		MaxIterations  *int     `json:"maxIterations"`
		CooldownSec    *int     `json:"cooldownSec"`
		ExpiresAt      *int64   `json:"expiresAt"`
		Enabled        *bool    `json:"enabled"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErr(err)
	}
	in.TriggerKind = strings.TrimSpace(in.TriggerKind)
	in.TriggerTag = strings.TrimSpace(in.TriggerTag)
	in.BoardOp = strings.TrimSpace(in.BoardOp)
	in.BoardFromState = strings.TrimSpace(in.BoardFromState)
	in.BoardToState = strings.TrimSpace(in.BoardToState)
	in.TargetAgentID = strings.TrimSpace(in.TargetAgentID)
	in.FlowID = strings.TrimSpace(in.FlowID)
	if strings.TrimSpace(in.PromptTemplate) == "" {
		return "", fmt.Errorf("promptTemplate is required")
	}
	if in.TriggerKind == db.TriggerBoard {
		if !db.ValidBoardOp(in.BoardOp) {
			return "", fmt.Errorf("invalid boardOp %q (any|move|create|update|delete)", in.BoardOp)
		}
	} else if in.TriggerTag == "" {
		return "", fmt.Errorf("triggerTag is required for tag automations")
	}
	// Either a flow (flowId) or an agent (targetAgentId) is the target.
	if in.FlowID != "" {
		if _, err := t.d.db.GetFlow(ctx, in.FlowID); err != nil {
			return "", fmt.Errorf("no flow with id %q (use list_flows)", in.FlowID)
		}
	} else {
		if in.TargetAgentID == "" {
			return "", fmt.Errorf("targetAgentId or flowId is required")
		}
		if _, err := t.d.db.GetAgent(ctx, in.TargetAgentID); err != nil {
			return "", fmt.Errorf("no agent with id %q (use list_agents)", in.TargetAgentID)
		}
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
	var expiresAt int64
	if in.ExpiresAt != nil {
		expiresAt = *in.ExpiresAt
	}
	created, err := t.d.db.CreateAutomation(ctx, db.Automation{
		Name:           strings.TrimSpace(in.Name),
		TriggerKind:    in.TriggerKind,
		TriggerTag:     in.TriggerTag,
		BoardOp:        in.BoardOp,
		BoardFromState: in.BoardFromState,
		BoardToState:   in.BoardToState,
		TargetAgentID:  in.TargetAgentID,
		FlowID:         in.FlowID,
		PromptTemplate: in.PromptTemplate,
		SpawnTags:      in.SpawnTags,
		MaxIterations:  maxIter,
		CooldownSec:    cooldown,
		ExpiresAt:      expiresAt,
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
		Description: "Edit an agent-created automation (not one made by the user). Pass the id and the fields to change (name, triggerTag, targetAgentId, flowId, promptTemplate, spawnTags, maxIterations, cooldownSec, enabled). Setting flowId makes it flow-backed (and clears the agent); setting targetAgentId switches it back to agent-backed.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"id":{"type":"string","description":"The automation id (see list_automations)"},
				"name":{"type":"string"},
				"triggerKind":{"type":"string","enum":["tag","board"]},
				"triggerTag":{"type":"string"},
				"boardOp":{"type":"string","enum":["any","move","create","update","delete"]},
				"boardFromState":{"type":"string","description":"[board kind] source-column filter (empty = any)"},
				"boardToState":{"type":"string","description":"[board kind] target-column filter (empty = any)"},
				"targetAgentId":{"type":"string"},
				"flowId":{"type":"string","description":"Run this flow with the rendered prompt as input instead of spawning an agent session (see list_flows). Setting it clears the agent."},
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
		TriggerKind    *string   `json:"triggerKind"`
		TriggerTag     *string   `json:"triggerTag"`
		BoardOp        *string   `json:"boardOp"`
		BoardFromState *string   `json:"boardFromState"`
		BoardToState   *string   `json:"boardToState"`
		TargetAgentID  *string   `json:"targetAgentId"`
		FlowID         *string   `json:"flowId"`
		PromptTemplate *string   `json:"promptTemplate"`
		SpawnTags      *[]string `json:"spawnTags"`
		MaxIterations  *int      `json:"maxIterations"`
		CooldownSec    *int      `json:"cooldownSec"`
		ExpiresAt      *int64    `json:"expiresAt"`
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
	if in.TriggerKind != nil {
		cur.TriggerKind = strings.TrimSpace(*in.TriggerKind)
	}
	if in.TriggerTag != nil {
		cur.TriggerTag = strings.TrimSpace(*in.TriggerTag)
	}
	if in.BoardOp != nil {
		if op := strings.TrimSpace(*in.BoardOp); db.ValidBoardOp(op) {
			cur.BoardOp = op
		} else {
			return "", fmt.Errorf("invalid boardOp %q (any|move|create|update|delete)", op)
		}
	}
	if in.BoardFromState != nil {
		cur.BoardFromState = strings.TrimSpace(*in.BoardFromState)
	}
	if in.BoardToState != nil {
		cur.BoardToState = strings.TrimSpace(*in.BoardToState)
	}
	// A non-empty flowId switches to flow-backed (and clears the agent); an
	// explicit targetAgentId switches back to agent-backed (and clears the flow).
	if in.FlowID != nil && strings.TrimSpace(*in.FlowID) != "" {
		fid := strings.TrimSpace(*in.FlowID)
		if _, err := t.d.db.GetFlow(ctx, fid); err != nil {
			return "", fmt.Errorf("no flow with id %q (use list_flows)", fid)
		}
		cur.FlowID = fid
		cur.TargetAgentID = ""
	} else if in.TargetAgentID != nil && strings.TrimSpace(*in.TargetAgentID) != "" {
		if _, err := t.d.db.GetAgent(ctx, *in.TargetAgentID); err != nil {
			return "", fmt.Errorf("no agent with id %q", *in.TargetAgentID)
		}
		cur.TargetAgentID = *in.TargetAgentID
		cur.FlowID = ""
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
	if in.ExpiresAt != nil {
		cur.ExpiresAt = *in.ExpiresAt
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
		TriggerKind    string `json:"triggerKind"`
		TriggerTag     string `json:"triggerTag,omitempty"`
		BoardOp        string `json:"boardOp,omitempty"`
		BoardToState   string `json:"boardToState,omitempty"`
		TargetAgentID  string `json:"targetAgentId"`
		FlowID         string `json:"flowId,omitempty"`
		Enabled        bool   `json:"enabled"`
		IterationCount int    `json:"iterationCount"`
		MaxIterations  int    `json:"maxIterations"`
		CreatedByAgent bool   `json:"createdByAgent"`
	}
	out := make([]row, 0, len(autos))
	for _, a := range autos {
		kind := a.TriggerKind
		if kind == "" {
			kind = db.TriggerTag
		}
		out = append(out, row{
			ID:             a.ID,
			Name:           a.Name,
			TriggerKind:    kind,
			TriggerTag:     a.TriggerTag,
			BoardOp:        a.BoardOp,
			BoardToState:   a.BoardToState,
			TargetAgentID:  a.TargetAgentID,
			FlowID:         a.FlowID,
			Enabled:        a.Enabled,
			IterationCount: a.IterationCount,
			MaxIterations:  a.MaxIterations,
			CreatedByAgent: a.CreatedBy != "",
		})
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}
