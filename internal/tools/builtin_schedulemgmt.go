package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/providers"
)

// Schedule self-management tools let an agent create, edit, delete and list
// cron schedules (routines) in its workspace. Provenance is enforced: an agent
// may only edit/delete schedules it (or another agent) created — never ones the
// user made in the UI. After any change the cron scheduler is reloaded so it
// takes effect immediately.

type scheduleDeps struct {
	db      *db.DB
	actorID string
	reload  func(ctx context.Context) error
}

func (d scheduleDeps) requireScheduleCreatedByAgent(ctx context.Context, id string) (db.Schedule, error) {
	sc, err := d.db.GetSchedule(ctx, id)
	if err != nil {
		return db.Schedule{}, fmt.Errorf("no schedule with id %q (use list_schedules)", id)
	}
	if sc.CreatedBy == "" {
		return db.Schedule{}, fmt.Errorf("schedule %q was created by the user and cannot be edited or deleted by an agent", id)
	}
	return sc, nil
}

// CreateScheduleTool creates a cron schedule that delivers a prompt to an agent.
type CreateScheduleTool struct{ d scheduleDeps }

// NewCreateScheduleTool constructs create_schedule.
func NewCreateScheduleTool(database *db.DB, actorID string, reload func(context.Context) error) CreateScheduleTool {
	return CreateScheduleTool{d: scheduleDeps{db: database, actorID: actorID, reload: reload}}
}

func (CreateScheduleTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "create_schedule",
		Description: "Create a recurring schedule (routine) that delivers a prompt to an agent on a cron expression. Example cronExpr: \"0 9 * * *\" (every day at 09:00), \"*/30 * * * *\" (every 30 minutes). The schedule is tagged as created by you. Returns the new schedule id.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"agentId":{"type":"string","description":"The agent that receives the prompt when the schedule fires (see list_agents)"},
				"cronExpr":{"type":"string","description":"Standard 5-field cron expression, e.g. \"0 9 * * *\""},
				"prompt":{"type":"string","description":"The prompt delivered to the agent on each fire"},
				"enabled":{"type":"boolean","description":"Whether the schedule is active immediately (default true)"}
			},
			"required":["agentId","cronExpr","prompt"],
			"additionalProperties":false
		}`),
	}
}

func (t CreateScheduleTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		AgentID  string `json:"agentId"`
		CronExpr string `json:"cronExpr"`
		Prompt   string `json:"prompt"`
		Enabled  *bool  `json:"enabled"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	in.AgentID = strings.TrimSpace(in.AgentID)
	in.CronExpr = strings.TrimSpace(in.CronExpr)
	in.Prompt = strings.TrimSpace(in.Prompt)
	if in.AgentID == "" || in.CronExpr == "" || in.Prompt == "" {
		return "", fmt.Errorf("agentId, cronExpr and prompt are all required")
	}
	if _, err := t.d.db.GetAgent(ctx, in.AgentID); err != nil {
		return "", fmt.Errorf("no agent with id %q (use list_agents)", in.AgentID)
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	created, err := t.d.db.CreateSchedule(ctx, db.Schedule{
		AgentID:   in.AgentID,
		CronExpr:  in.CronExpr,
		Prompt:    in.Prompt,
		Enabled:   enabled,
		CreatedBy: t.d.actorID,
	})
	if err != nil {
		return "", fmt.Errorf("create schedule: %w", err)
	}
	if err := t.d.reload(ctx); err != nil {
		return "", fmt.Errorf("schedule saved (%s) but reload failed (check cron syntax): %w", created.ID, err)
	}
	b, _ := json.Marshal(map[string]any{"id": created.ID, "enabled": enabled, "action": "created"})
	return string(b), nil
}

// UpdateScheduleTool edits an agent-created schedule.
type UpdateScheduleTool struct{ d scheduleDeps }

// NewUpdateScheduleTool constructs update_schedule.
func NewUpdateScheduleTool(database *db.DB, actorID string, reload func(context.Context) error) UpdateScheduleTool {
	return UpdateScheduleTool{d: scheduleDeps{db: database, actorID: actorID, reload: reload}}
}

func (UpdateScheduleTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "update_schedule",
		Description: "Edit an agent-created schedule (not one made by the user). Pass the schedule id and the fields to change (agentId, cronExpr, prompt, enabled).",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"id":{"type":"string","description":"The schedule id (see list_schedules)"},
				"agentId":{"type":"string"},
				"cronExpr":{"type":"string"},
				"prompt":{"type":"string"},
				"enabled":{"type":"boolean"}
			},
			"required":["id"],
			"additionalProperties":false
		}`),
	}
}

func (t UpdateScheduleTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		ID       string  `json:"id"`
		AgentID  *string `json:"agentId"`
		CronExpr *string `json:"cronExpr"`
		Prompt   *string `json:"prompt"`
		Enabled  *bool   `json:"enabled"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	in.ID = strings.TrimSpace(in.ID)
	if in.ID == "" {
		return "", fmt.Errorf("id is required")
	}
	cur, err := t.d.requireScheduleCreatedByAgent(ctx, in.ID)
	if err != nil {
		return "", err
	}
	if in.AgentID != nil {
		if _, err := t.d.db.GetAgent(ctx, *in.AgentID); err != nil {
			return "", fmt.Errorf("no agent with id %q", *in.AgentID)
		}
		cur.AgentID = *in.AgentID
	}
	if in.CronExpr != nil {
		cur.CronExpr = strings.TrimSpace(*in.CronExpr)
	}
	if in.Prompt != nil {
		cur.Prompt = *in.Prompt
	}
	if err := t.d.db.UpdateSchedule(ctx, cur); err != nil {
		return "", fmt.Errorf("update schedule: %w", err)
	}
	if in.Enabled != nil {
		if err := t.d.db.SetScheduleEnabled(ctx, in.ID, *in.Enabled); err != nil {
			return "", fmt.Errorf("set enabled: %w", err)
		}
	}
	if err := t.d.reload(ctx); err != nil {
		return "", fmt.Errorf("schedule updated but reload failed (check cron syntax): %w", err)
	}
	b, _ := json.Marshal(map[string]string{"id": in.ID, "action": "updated"})
	return string(b), nil
}

// DeleteScheduleTool removes an agent-created schedule.
type DeleteScheduleTool struct{ d scheduleDeps }

// NewDeleteScheduleTool constructs delete_schedule.
func NewDeleteScheduleTool(database *db.DB, actorID string, reload func(context.Context) error) DeleteScheduleTool {
	return DeleteScheduleTool{d: scheduleDeps{db: database, actorID: actorID, reload: reload}}
}

func (DeleteScheduleTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "delete_schedule",
		Description: "Delete an agent-created schedule (not one made by the user). Pass the schedule id.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{"id":{"type":"string","description":"The schedule id (see list_schedules)"}},
			"required":["id"],
			"additionalProperties":false
		}`),
	}
}

func (t DeleteScheduleTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
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
	if _, err := t.d.requireScheduleCreatedByAgent(ctx, in.ID); err != nil {
		return "", err
	}
	if err := t.d.db.DeleteSchedule(ctx, in.ID); err != nil {
		return "", fmt.Errorf("delete schedule: %w", err)
	}
	if err := t.d.reload(ctx); err != nil {
		return "", fmt.Errorf("schedule deleted but reload failed: %w", err)
	}
	b, _ := json.Marshal(map[string]string{"id": in.ID, "action": "deleted"})
	return string(b), nil
}

// ListSchedulesTool lists the workspace schedules with provenance.
type ListSchedulesTool struct{ d scheduleDeps }

// NewListSchedulesTool constructs list_schedules.
func NewListSchedulesTool(database *db.DB, actorID string) ListSchedulesTool {
	return ListSchedulesTool{d: scheduleDeps{db: database, actorID: actorID}}
}

func (ListSchedulesTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "list_schedules",
		Description: "List the schedules (routines) in this workspace (id, agent, cron, prompt, enabled, and whether each was created by an agent and is therefore editable/deletable by you).",
		InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
	}
}

func (t ListSchedulesTool) Call(ctx context.Context, _ json.RawMessage) (string, error) {
	schedules, err := t.d.db.ListSchedules(ctx)
	if err != nil {
		return "", err
	}
	type row struct {
		ID             string `json:"id"`
		AgentID        string `json:"agentId"`
		CronExpr       string `json:"cronExpr"`
		Prompt         string `json:"prompt"`
		Enabled        bool   `json:"enabled"`
		CreatedByAgent bool   `json:"createdByAgent"`
	}
	out := make([]row, 0, len(schedules))
	for _, sc := range schedules {
		// One-shot wakes (schedule_wake) are transient, not routines — hide them.
		if sc.OneShot {
			continue
		}
		out = append(out, row{
			ID:             sc.ID,
			AgentID:        sc.AgentID,
			CronExpr:       sc.CronExpr,
			Prompt:         sc.Prompt,
			Enabled:        sc.Enabled,
			CreatedByAgent: sc.CreatedBy != "",
		})
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}
