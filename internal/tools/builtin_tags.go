package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/db"
	"github.com/bilal-arikan/swarmgo/internal/providers"
)

// Tag self-management tools let an agent edit the free-form tags on sessions,
// flows and schedules — the same tags the user edits in the UI. Session tags
// additionally drive tag-triggered automations, so an agent can enrol a session
// into (or out of) an automation loop by tagging it.

// --- set_session_tags (current session, via sink) ---

// SetSessionTagsTool edits THIS session's tags. It supports full replacement
// ("tags") or incremental changes ("add"/"remove"), reading the current set from
// the sink first. No-op without a session sink.
type SetSessionTagsTool struct{}

// NewSetSessionTagsTool constructs the set_session_tags tool.
func NewSetSessionTagsTool() SetSessionTagsTool { return SetSessionTagsTool{} }

func (SetSessionTagsTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "set_session_tags",
		Description: "Edit this chat session's tags — free-form labels shared with the user's UI. " +
			"Pass \"tags\" to replace the whole set, or \"add\"/\"remove\" to change it incrementally. " +
			"Session tags also drive automations: tagging a session with an automation's trigger tag " +
			"enrols it, so when this session's turn finishes the automation fires.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "tags":   { "type": "array", "items": { "type": "string" }, "description": "Replace all tags with this exact set." },
    "add":    { "type": "array", "items": { "type": "string" }, "description": "Tags to add (kept alongside existing ones)." },
    "remove": { "type": "array", "items": { "type": "string" }, "description": "Tags to remove." }
  },
  "additionalProperties": false
}`),
		Examples: []json.RawMessage{
			json.RawMessage(`{"add":["loop"]}`),
			json.RawMessage(`{"tags":["research","priority"]}`),
			json.RawMessage(`{"remove":["loop"]}`),
		},
	}
}

func (SetSessionTagsTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Tags   *[]string `json:"tags"`
		Add    []string  `json:"add"`
		Remove []string  `json:"remove"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErrFor("set_session_tags", err)
	}
	sink := sessionFrom(ctx)
	if sink == nil {
		return "no session is available to tag for this turn", nil
	}
	var next []string
	if in.Tags != nil {
		next = *in.Tags
	} else {
		cur, err := sink.Tags(ctx)
		if err != nil {
			return "", err
		}
		next = applyTagDelta(cur, in.Add, in.Remove)
	}
	if err := sink.SetTags(ctx, next); err != nil {
		return "", err
	}
	b, _ := json.Marshal(map[string]any{"tags": next})
	return string(b), nil
}

// applyTagDelta returns cur with add-ed tags appended (de-duplicated) and
// remove-d tags dropped, preserving order.
func applyTagDelta(cur, add, remove []string) []string {
	rm := make(map[string]struct{}, len(remove))
	for _, t := range remove {
		rm[strings.TrimSpace(t)] = struct{}{}
	}
	seen := make(map[string]struct{})
	out := make([]string, 0, len(cur)+len(add))
	appendTag := func(t string) {
		t = strings.TrimSpace(t)
		if t == "" {
			return
		}
		if _, drop := rm[t]; drop {
			return
		}
		if _, dup := seen[t]; dup {
			return
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	for _, t := range cur {
		appendTag(t)
	}
	for _, t := range add {
		appendTag(t)
	}
	return out
}

// --- set_flow_tags / set_schedule_tags (by id, via db) ---

// SetFlowTagsTool replaces a flow's tags by id (organizational; not provenance-
// restricted since tagging is non-destructive).
type SetFlowTagsTool struct{ db *db.DB }

// NewSetFlowTagsTool constructs set_flow_tags.
func NewSetFlowTagsTool(database *db.DB) SetFlowTagsTool { return SetFlowTagsTool{db: database} }

func (SetFlowTagsTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "set_flow_tags",
		Description: "Replace the tags on a flow (organizational labels shared with the UI). Use list_flows to find the id.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"id":{"type":"string","description":"The flow id (see list_flows)"},
				"tags":{"type":"array","items":{"type":"string"},"description":"The full replacement tag set"}
			},
			"required":["id","tags"],
			"additionalProperties":false
		}`),
	}
}

func (t SetFlowTagsTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		ID   string   `json:"id"`
		Tags []string `json:"tags"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErrFor("set_flow_tags", err)
	}
	in.ID = strings.TrimSpace(in.ID)
	if in.ID == "" {
		return "", fmt.Errorf("id is required")
	}
	if _, err := t.db.GetFlow(ctx, in.ID); err != nil {
		return "", fmt.Errorf("no flow with id %q (use list_flows)", in.ID)
	}
	if err := t.db.SetFlowTags(ctx, in.ID, in.Tags); err != nil {
		return "", fmt.Errorf("set flow tags: %w", err)
	}
	b, _ := json.Marshal(map[string]any{"id": in.ID, "tags": in.Tags})
	return string(b), nil
}

// SetScheduleTagsTool replaces a schedule's tags by id.
type SetScheduleTagsTool struct{ db *db.DB }

// NewSetScheduleTagsTool constructs set_schedule_tags.
func NewSetScheduleTagsTool(database *db.DB) SetScheduleTagsTool { return SetScheduleTagsTool{db: database} }

func (SetScheduleTagsTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "set_schedule_tags",
		Description: "Replace the tags on a schedule (organizational labels shared with the UI). Use list_schedules to find the id.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"id":{"type":"string","description":"The schedule id (see list_schedules)"},
				"tags":{"type":"array","items":{"type":"string"},"description":"The full replacement tag set"}
			},
			"required":["id","tags"],
			"additionalProperties":false
		}`),
	}
}

func (t SetScheduleTagsTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		ID   string   `json:"id"`
		Tags []string `json:"tags"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErrFor("set_schedule_tags", err)
	}
	in.ID = strings.TrimSpace(in.ID)
	if in.ID == "" {
		return "", fmt.Errorf("id is required")
	}
	if _, err := t.db.GetSchedule(ctx, in.ID); err != nil {
		return "", fmt.Errorf("no schedule with id %q (use list_schedules)", in.ID)
	}
	if err := t.db.SetScheduleTags(ctx, in.ID, in.Tags); err != nil {
		return "", fmt.Errorf("set schedule tags: %w", err)
	}
	b, _ := json.Marshal(map[string]any{"id": in.ID, "tags": in.Tags})
	return string(b), nil
}
