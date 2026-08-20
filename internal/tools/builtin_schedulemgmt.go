package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
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

// requireSchedule loads a schedule by id, returning a friendly error if it does
// not exist. No provenance gate: user- and agent-created schedules are both editable.
func (d scheduleDeps) requireSchedule(ctx context.Context, id string) (db.Schedule, error) {
	sc, err := d.db.GetSchedule(ctx, id)
	if err != nil {
		return db.Schedule{}, fmt.Errorf("no schedule with id %q (use list_schedules)", id)
	}
	return sc, nil
}

// RunScheduleTool fires a schedule immediately on demand — the manual "Run now"
// trigger — regardless of its cron timing or enabled state, so an agent can
// kick off a routine itself. Runs synchronously: the schedule's prompt is
// delivered to its agent. Works on any schedule (running is not destructive, so
// provenance is not enforced — unlike edit/delete).
type RunScheduleTool struct {
	db  *db.DB
	run func(ctx context.Context, scheduleID string) error
}

// NewRunScheduleTool constructs run_schedule over the scheduler's RunNow.
func NewRunScheduleTool(database *db.DB, run func(context.Context, string) error) RunScheduleTool {
	return RunScheduleTool{db: database, run: run}
}

func (RunScheduleTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "run_schedule",
		Description: "Fire a schedule immediately, regardless of its cron timing or enabled state — the manual \"Run now\" trigger. Delivers the schedule's prompt to its agent synchronously. Use list_schedules to find the id. Works on any schedule in the workspace.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"id":{"type":"string","description":"The schedule id to fire now (see list_schedules)"}
			},
			"required":["id"],
			"additionalProperties":false
		}`),
	}
}

func (t RunScheduleTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
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
	sc, err := t.db.GetSchedule(ctx, in.ID)
	if err != nil {
		return "", fmt.Errorf("no schedule with id %q (use list_schedules)", in.ID)
	}
	if t.run == nil {
		return "", fmt.Errorf("schedule runner is not available in this context")
	}
	if err := t.run(ctx, in.ID); err != nil {
		return "", fmt.Errorf("run schedule: %w", err)
	}
	b, _ := json.Marshal(map[string]string{"id": sc.ID, "action": "fired"})
	return string(b), nil
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
		Description: "Create a recurring schedule (routine) on a cron expression. It either delivers a prompt to an agent (agentId+prompt) OR runs an orchestration flow (flowId, with prompt as the flow input). New schedules default to disabled, matching REST. The schedule is tagged as created by you. Returns the new schedule id.",
		InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{
					"name":{"type":"string","description":"A short label for the schedule, e.g. \"Haftalık rapor\"."},
					"agentId":{"type":"string","description":"The agent that receives the prompt when the schedule fires (see list_agents). Omit when flowId is set."},
					"flowId":{"type":"string","description":"Run this orchestration flow on each fire instead of delivering the prompt to an agent (see list_flows). prompt becomes the flow input."},
					"cronExpr":{"type":"string","description":"Standard 5-field cron expression, e.g. \"0 9 * * *\""},
					"prompt":{"type":"string","description":"The prompt delivered to the agent on each fire (or the flow input when flowId is set)"},
					"enabled":{"type":"boolean","description":"Whether the schedule is active immediately (default false)"},
					"expiresAt":{"type":"integer","description":"Optional end date (unix seconds); 0 means no end date"}
				},
				"required":["cronExpr"],
				"additionalProperties":false
			}`),
		Examples: []json.RawMessage{
			// Minimal: required fields only; standard 5-field cron (daily 09:00).
			json.RawMessage(`{"agentId":"agt_7f3a","cronExpr":"0 9 * * *","prompt":"Summarise overnight changes and post them to the team."}`),
			// Weekday business hours, every 30 min, created paused (enabled:false).
			json.RawMessage(`{"agentId":"agt_7f3a","cronExpr":"*/30 9-18 * * 1-5","prompt":"Check the build queue; flag anything stuck.","enabled":false}`),
		},
	}
}

func (t CreateScheduleTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Name      string `json:"name"`
		AgentID   string `json:"agentId"`
		FlowID    string `json:"flowId"`
		CronExpr  string `json:"cronExpr"`
		Prompt    string `json:"prompt"`
		Enabled   *bool  `json:"enabled"`
		ExpiresAt int64  `json:"expiresAt"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErr(err)
	}
	in.AgentID = strings.TrimSpace(in.AgentID)
	in.FlowID = strings.TrimSpace(in.FlowID)
	in.CronExpr = strings.TrimSpace(in.CronExpr)
	in.Prompt = strings.TrimSpace(in.Prompt)
	if in.CronExpr == "" {
		return "", fmt.Errorf("cronExpr is required")
	}
	// Either a flow (flowId) or an agent+prompt drives the schedule.
	if in.FlowID != "" {
		if _, err := t.d.db.GetFlow(ctx, in.FlowID); err != nil {
			return "", fmt.Errorf("no flow with id %q (use list_flows)", in.FlowID)
		}
	} else {
		if in.AgentID == "" || in.Prompt == "" {
			return "", fmt.Errorf("agentId and prompt are required (or provide flowId)")
		}
		if _, err := t.d.db.GetAgent(ctx, in.AgentID); err != nil {
			return "", fmt.Errorf("no agent with id %q (use list_agents)", in.AgentID)
		}
	}
	enabled := false
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	created, err := t.d.db.CreateSchedule(ctx, db.Schedule{
		Name:      in.Name,
		AgentID:   in.AgentID,
		FlowID:    in.FlowID,
		CronExpr:  in.CronExpr,
		Prompt:    in.Prompt,
		Enabled:   enabled,
		ExpiresAt: in.ExpiresAt,
		CreatedBy: t.d.actorID,
	})
	if err != nil {
		return "", fmt.Errorf("create schedule: %w", err)
	}
	if err := t.d.reload(ctx); err != nil {
		return "", fmt.Errorf("schedule saved (%s) but reload failed: %w. %s", created.ID, err, cronHint)
	}
	b, _ := json.Marshal(map[string]any{"id": created.ID, "enabled": enabled, "action": "created"})
	return string(b), nil
}

// UpdateScheduleTool edits a schedule (user- or agent-created).
type UpdateScheduleTool struct{ d scheduleDeps }

// NewUpdateScheduleTool constructs update_schedule.
func NewUpdateScheduleTool(database *db.DB, actorID string, reload func(context.Context) error) UpdateScheduleTool {
	return UpdateScheduleTool{d: scheduleDeps{db: database, actorID: actorID, reload: reload}}
}

func (UpdateScheduleTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "update_schedule",
		Description: "Edit a schedule (user- or agent-created). Pass the schedule id and the fields to change (name, agentId, flowId, cronExpr, prompt, enabled, tags, expiresAt). Setting flowId makes it flow-backed (and clears the agent); setting agentId switches it back to prompt delivery.",
		InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{
					"id":{"type":"string","description":"The schedule id (see list_schedules)"},
					"name":{"type":"string","description":"Rename the schedule."},
					"agentId":{"type":"string"},
					"flowId":{"type":"string","description":"Run this flow on each fire instead of an agent prompt (see list_flows). Setting it clears the agent."},
					"cronExpr":{"type":"string"},
					"prompt":{"type":"string"},
					"enabled":{"type":"boolean"},
					"expiresAt":{"type":"integer","description":"Optional end date (unix seconds); 0 means no end date"},
					"tags":{"type":"array","items":{"type":"string"},"description":"Replace the schedule's organizational tags with this exact set"}
				},
				"required":["id"],
				"additionalProperties":false
			}`),
		Examples: []json.RawMessage{
			// Partial update: pause a schedule without touching its cron/prompt.
			json.RawMessage(`{"id":"sch_4f1","enabled":false}`),
			// Change only the cron (every 2 hours).
			json.RawMessage(`{"id":"sch_4f1","cronExpr":"0 */2 * * *"}`),
		},
	}
}

func (t UpdateScheduleTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		ID        string    `json:"id"`
		Name      *string   `json:"name"`
		AgentID   *string   `json:"agentId"`
		FlowID    *string   `json:"flowId"`
		CronExpr  *string   `json:"cronExpr"`
		Prompt    *string   `json:"prompt"`
		Enabled   *bool     `json:"enabled"`
		ExpiresAt *int64    `json:"expiresAt"`
		Tags      *[]string `json:"tags"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErr(err)
	}
	in.ID = strings.TrimSpace(in.ID)
	if in.ID == "" {
		return "", fmt.Errorf("id is required")
	}
	cur, err := t.d.requireSchedule(ctx, in.ID)
	if err != nil {
		return "", err
	}
	// A non-empty flowId switches to flow-backed (and clears the agent); an
	// explicit agentId switches back to prompt delivery (and clears the flow).
	if in.FlowID != nil && strings.TrimSpace(*in.FlowID) != "" {
		fid := strings.TrimSpace(*in.FlowID)
		if _, err := t.d.db.GetFlow(ctx, fid); err != nil {
			return "", fmt.Errorf("no flow with id %q (use list_flows)", fid)
		}
		cur.FlowID = fid
		cur.AgentID = ""
	} else if in.AgentID != nil && strings.TrimSpace(*in.AgentID) != "" {
		if _, err := t.d.db.GetAgent(ctx, *in.AgentID); err != nil {
			return "", fmt.Errorf("no agent with id %q", *in.AgentID)
		}
		cur.AgentID = *in.AgentID
		cur.FlowID = ""
	}
	if in.Name != nil {
		cur.Name = strings.TrimSpace(*in.Name)
	}
	if in.CronExpr != nil {
		// Reject an empty cron on update, mirroring create — a blank cron would
		// leave the schedule with no valid fire time.
		expr := strings.TrimSpace(*in.CronExpr)
		if expr == "" {
			return "", fmt.Errorf("cronExpr cannot be empty")
		}
		cur.CronExpr = expr
	}
	if in.Prompt != nil {
		cur.Prompt = *in.Prompt
	}
	if in.ExpiresAt != nil {
		cur.ExpiresAt = *in.ExpiresAt
	}
	if err := t.d.db.UpdateSchedule(ctx, cur); err != nil {
		return "", fmt.Errorf("update schedule: %w", err)
	}
	if in.Enabled != nil {
		if err := t.d.db.SetScheduleEnabled(ctx, in.ID, *in.Enabled); err != nil {
			return "", fmt.Errorf("set enabled: %w", err)
		}
	}
	if in.Tags != nil {
		if err := t.d.db.SetScheduleTags(ctx, in.ID, *in.Tags); err != nil {
			return "", fmt.Errorf("set schedule tags: %w", err)
		}
	}
	if err := t.d.reload(ctx); err != nil {
		return "", fmt.Errorf("schedule updated but reload failed: %w. %s", err, cronHint)
	}
	b, _ := json.Marshal(map[string]string{"id": in.ID, "action": "updated"})
	return string(b), nil
}

// DeleteScheduleTool removes a schedule (user- or agent-created).
type DeleteScheduleTool struct{ d scheduleDeps }

// NewDeleteScheduleTool constructs delete_schedule.
func NewDeleteScheduleTool(database *db.DB, actorID string, reload func(context.Context) error) DeleteScheduleTool {
	return DeleteScheduleTool{d: scheduleDeps{db: database, actorID: actorID, reload: reload}}
}

func (DeleteScheduleTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "delete_schedule",
		Description: "Delete a schedule (user- or agent-created). Pass the schedule id.",
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
		return "", argErr(err)
	}
	in.ID = strings.TrimSpace(in.ID)
	if in.ID == "" {
		return "", fmt.Errorf("id is required")
	}
	if _, err := t.d.requireSchedule(ctx, in.ID); err != nil {
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
		Name: "list_schedules",
		Description: "List the schedules (routines) in this workspace (id, name, agent, flowId, cron, prompt, enabled, expiresAt, " +
			"and whether each was created by an agent — provenance only; you can edit/delete any of them). " +
			"One-shot schedule_wake entries are transient, not routines, so they are excluded (this also shapes total/hasMore). " +
			"Results are PAGINATED: pass limit (default 20, max 100) and offset to page; the reply reports total " +
			"and hasMore, and you reach the next page with offset += limit. Filters: enabled (true/false), " +
			"agentId (exact). Sort: updated_desc (default), updated_asc, created_desc, created_asc, name_asc, " +
			"name_desc.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "enabled": { "type": "boolean", "description": "Only enabled (true) or disabled (false) schedules." },
    "agentId": { "type": "string", "description": "Only schedules delivering to this agent." },
    "sort": { "type": "string", "enum": ["updated_desc", "updated_asc", "created_desc", "created_asc", "name_asc", "name_desc"], "description": "Result ordering (default updated_desc)." },
    "limit": { "type": "integer", "description": "Max schedules per page (default 20, max 100)." },
    "offset": { "type": "integer", "description": "How many matching schedules to skip before this page (default 0)." }
  },
  "additionalProperties": false
}`),
	}
}

func (t ListSchedulesTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Enabled *bool  `json:"enabled"`
		AgentID string `json:"agentId"`
		Sort    string `json:"sort"`
		Limit   int    `json:"limit"`
		Offset  int    `json:"offset"`
	}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &in); err != nil {
			return "", argErr(err)
		}
	}
	limit, offset := PageArgs(in.Limit, in.Offset)

	schedules, err := t.d.db.ListSchedules(ctx)
	if err != nil {
		return "", err
	}

	agentID := strings.TrimSpace(in.AgentID)
	matches := make([]db.Schedule, 0, len(schedules))
	for _, sc := range schedules {
		// One-shot wakes (schedule_wake) are transient, not routines — hide them.
		if sc.OneShot {
			continue
		}
		if in.Enabled != nil && sc.Enabled != *in.Enabled {
			continue
		}
		if agentID != "" && sc.AgentID != agentID {
			continue
		}
		matches = append(matches, sc)
	}

	field, asc, err := SortOrder(in.Sort)
	if err != nil {
		return "", err
	}
	less, err := SortByField(matches, field, asc,
		func(sc db.Schedule) int64 { return sc.UpdatedAt },
		func(sc db.Schedule) int64 { return sc.CreatedAt },
		func(sc db.Schedule) string { return sc.Name },
		func(sc db.Schedule) string { return sc.ID },
	)
	if err != nil {
		return "", err
	}
	sort.SliceStable(matches, less)

	page, total := SlicePage(matches, offset, limit)
	type row struct {
		ID             string `json:"id"`
		Name           string `json:"name,omitempty"`
		AgentID        string `json:"agentId"`
		FlowID         string `json:"flowId,omitempty"`
		CronExpr       string `json:"cronExpr"`
		Prompt         string `json:"prompt"`
		Enabled        bool   `json:"enabled"`
		ExpiresAt      int64  `json:"expiresAt,omitempty"`
		CreatedByAgent bool   `json:"createdByAgent"`
	}
	out := make([]row, 0, len(page))
	for _, sc := range page {
		out = append(out, row{
			ID:             sc.ID,
			Name:           sc.Name,
			AgentID:        sc.AgentID,
			FlowID:         sc.FlowID,
			CronExpr:       sc.CronExpr,
			Prompt:         sc.Prompt,
			Enabled:        sc.Enabled,
			ExpiresAt:      sc.ExpiresAt,
			CreatedByAgent: sc.CreatedBy != "",
		})
	}
	return pageResult(out, total, offset, limit)
}
