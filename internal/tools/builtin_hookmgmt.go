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

// Hook self-management tools let an agent read and manage PreToolUse/PostToolUse
// hooks in its workspace — external commands that intercept native tool calls
// (Claude Code hook contract). No provenance gate: list/create/delete act on any
// hook, user- or agent-created (Hook.CreatedBy is kept for provenance/display).

type hookDeps struct {
	db      *db.DB
	actorID string
}

// ---- list_hooks ----

// ListHooksTool returns the workspace hooks as a compact list.
type ListHooksTool struct{ d hookDeps }

// NewListHooksTool constructs list_hooks.
func NewListHooksTool(database *db.DB, actorID string) ListHooksTool {
	return ListHooksTool{d: hookDeps{db: database, actorID: actorID}}
}

func (ListHooksTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "list_hooks",
		Description: "List tool and lifecycle hooks in this workspace. Each hook runs an external command " +
			"around a native tool call (matched by a tool-name glob; empty = all tools). Returns id, event, " +
			"matcher, command, enabled, and whether you created it (and may therefore delete it). Results are " +
			"PAGINATED: pass limit (default 20, max 100) and offset to page; the reply reports total and hasMore, " +
			"and you reach the next page with offset += limit. Filter: event. Sort: " +
			"updated_desc (default), updated_asc, created_desc, created_asc — hooks are immutable after creation, " +
			"so updated_* sorts by creation time; they have no name field, so name_* is rejected with a clear error.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "event": { "type": "string", "enum": ["PreToolUse", "PostToolUse", "UserPromptSubmit", "SessionStart", "Stop", "SubagentStop", "PreCompact", "Notification", "SessionEnd"], "description": "Narrow to one hook event." },
    "sort": { "type": "string", "enum": ["updated_desc", "updated_asc", "created_desc", "created_asc"], "description": "Result ordering (default updated_desc; updated maps to creation time — hooks are immutable)." },
    "limit": { "type": "integer", "description": "Max hooks per page (default 20, max 100)." },
    "offset": { "type": "integer", "description": "How many matching hooks to skip before this page (default 0)." }
  },
  "additionalProperties": false
}`),
	}
}

func (t ListHooksTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Event  string `json:"event"`
		Sort   string `json:"sort"`
		Limit  int    `json:"limit"`
		Offset int    `json:"offset"`
	}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &in); err != nil {
			return "", argErr(err)
		}
	}
	limit, offset := PageArgs(in.Limit, in.Offset)

	hooks, err := t.d.db.ListHooks(ctx)
	if err != nil {
		return "", err
	}
	event := strings.TrimSpace(in.Event)
	matches := make([]db.Hook, 0, len(hooks))
	for _, h := range hooks {
		if event != "" && h.Event != event {
			continue
		}
		matches = append(matches, h)
	}

	field, asc, err := SortOrder(in.Sort)
	if err != nil {
		return "", err
	}
	// Hooks are never edited in place (no updated timestamp), so the updated_*
	// keys sort by CreatedAt — the same value as created_*. name_* is rejected by
	// SortByField because hooks have no name field.
	less, err := SortByField(matches, field, asc,
		func(h db.Hook) int64 { return h.CreatedAt },
		func(h db.Hook) int64 { return h.CreatedAt },
		nil,
		func(h db.Hook) string { return h.ID },
	)
	if err != nil {
		return "", err
	}
	sort.SliceStable(matches, less)

	page, total := SlicePage(matches, offset, limit)
	type row struct {
		ID             string `json:"id"`
		Event          string `json:"event"`
		Matcher        string `json:"matcher"`
		Command        string `json:"command"`
		Enabled        bool   `json:"enabled"`
		CreatedByAgent bool   `json:"createdByAgent"`
	}
	out := make([]row, 0, len(page))
	for _, h := range page {
		out = append(out, row{
			ID: h.ID, Event: h.Event, Matcher: h.Matcher,
			Command: truncateForTool(h.Command, 200), Enabled: h.Enabled,
			CreatedByAgent: h.CreatedBy != "",
		})
	}
	return pageResult(out, total, offset, limit)
}

// ---- create_hook ----

// CreateHookTool adds a hook to the workspace.
type CreateHookTool struct{ d hookDeps }

// NewCreateHookTool constructs create_hook.
func NewCreateHookTool(database *db.DB, actorID string) CreateHookTool {
	return CreateHookTool{d: hookDeps{db: database, actorID: actorID}}
}

func (CreateHookTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "create_hook",
		Description: "Create a tool or lifecycle hook. The command speaks the Claude Code hook contract (JSON on stdin, JSON decision on stdout). matcher narrows the event where supported. The hook is enabled and tagged as created by you. Returns the new hook id.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"event":{"type":"string","enum":["PreToolUse","PostToolUse","UserPromptSubmit","SessionStart","Stop","SubagentStop","PreCompact","Notification","SessionEnd"]},
				"matcher":{"type":"string","description":"Tool-name glob to match; empty = all tools"},
				"command":{"type":"string","description":"Shell command to run (receives the call as JSON on stdin)"},
				"timeoutSec":{"type":"integer","description":"Max seconds the command may run (0 = default)"}
			},
			"required":["event","command"],
			"additionalProperties":false
		}`),
		Examples: []json.RawMessage{
			// Pre-hook: block a shell call by exiting non-zero; reads the call JSON on stdin.
			json.RawMessage(`{"event":"PreToolUse","matcher":"Bash","command":"jq -e '.tool_input.command | test(\"rm -rf\") | not'"}`),
			// Post-hook scoped to one tool, with a timeout.
			json.RawMessage(`{"event":"PostToolUse","matcher":"Write","command":"prettier --write \"$(jq -r .tool_input.path)\"","timeoutSec":30}`),
		},
	}
}

func (t CreateHookTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Event      string `json:"event"`
		Matcher    string `json:"matcher"`
		Command    string `json:"command"`
		TimeoutSec int    `json:"timeoutSec"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErr(err)
	}
	if !db.ValidHookEvent(in.Event) {
		return "", fmt.Errorf("event must be one of: PreToolUse, PostToolUse, UserPromptSubmit, SessionStart, Stop, SubagentStop, PreCompact, Notification, SessionEnd")
	}
	if strings.TrimSpace(in.Command) == "" {
		return "", fmt.Errorf("command is required")
	}
	created, err := t.d.db.CreateHook(ctx, db.Hook{
		Event:      in.Event,
		Matcher:    in.Matcher,
		Type:       "command",
		Command:    in.Command,
		TimeoutSec: in.TimeoutSec,
		Enabled:    true,
		CreatedBy:  t.d.actorID,
	})
	if err != nil {
		return "", fmt.Errorf("create hook: %w", err)
	}
	b, _ := json.Marshal(map[string]string{"id": created.ID, "action": "created"})
	return string(b), nil
}

// UpdateHookTool edits a hook's REST-equivalent mutable field set.
type UpdateHookTool struct{ d hookDeps }

// NewUpdateHookTool constructs update_hook.
func NewUpdateHookTool(database *db.DB, actorID string) UpdateHookTool {
	return UpdateHookTool{d: hookDeps{db: database, actorID: actorID}}
}

func (UpdateHookTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "update_hook",
		Description: "Replace the mutable fields of a hook: event, matcher, command, timeoutSec, and enabled. This matches the REST update operation.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"id":{"type":"string","description":"The hook id (see list_hooks)"},
				"event":{"type":"string","enum":["PreToolUse","PostToolUse","UserPromptSubmit","SessionStart","Stop","SubagentStop","PreCompact","Notification","SessionEnd"]},
				"matcher":{"type":"string"},
				"command":{"type":"string"},
				"timeoutSec":{"type":"integer"},
				"enabled":{"type":"boolean"}
			},
			"required":["id","event","command","enabled"],
			"additionalProperties":false
		}`),
	}
}

func (t UpdateHookTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		ID         string `json:"id"`
		Event      string `json:"event"`
		Matcher    string `json:"matcher"`
		Command    string `json:"command"`
		TimeoutSec int    `json:"timeoutSec"`
		Enabled    bool   `json:"enabled"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErr(err)
	}
	in.ID = strings.TrimSpace(in.ID)
	if in.ID == "" {
		return "", fmt.Errorf("id is required")
	}
	if !db.ValidHookEvent(in.Event) {
		return "", fmt.Errorf("event must be one of: PreToolUse, PostToolUse, UserPromptSubmit, SessionStart, Stop, SubagentStop, PreCompact, Notification, SessionEnd")
	}
	if strings.TrimSpace(in.Command) == "" {
		return "", fmt.Errorf("command is required")
	}
	if _, err := t.d.db.GetHook(ctx, in.ID); err != nil {
		return "", fmt.Errorf("no hook with id %q (use list_hooks)", in.ID)
	}
	if err := t.d.db.UpdateHook(ctx, db.Hook{ID: in.ID, Event: in.Event, Matcher: in.Matcher, Type: "command", Command: in.Command, TimeoutSec: in.TimeoutSec, Enabled: in.Enabled}); err != nil {
		return "", fmt.Errorf("update hook: %w", err)
	}
	b, _ := json.Marshal(map[string]string{"id": in.ID, "action": "updated"})
	return string(b), nil
}

// ---- delete_hook ----

// DeleteHookTool removes a hook (user- or agent-created).
type DeleteHookTool struct{ d hookDeps }

// NewDeleteHookTool constructs delete_hook.
func NewDeleteHookTool(database *db.DB, actorID string) DeleteHookTool {
	return DeleteHookTool{d: hookDeps{db: database, actorID: actorID}}
}

func (DeleteHookTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "delete_hook",
		Description: "Delete a hook (user- or agent-created). Pass the hook id.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{"id":{"type":"string","description":"The hook id (see list_hooks)"}},
			"required":["id"],
			"additionalProperties":false
		}`),
	}
}

func (t DeleteHookTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
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
	if _, err := t.d.db.GetHook(ctx, in.ID); err != nil {
		return "", fmt.Errorf("no hook with id %q (use list_hooks)", in.ID)
	}
	if err := t.d.db.DeleteHook(ctx, in.ID); err != nil {
		return "", fmt.Errorf("delete hook: %w", err)
	}
	b, _ := json.Marshal(map[string]string{"id": in.ID, "action": "deleted"})
	return string(b), nil
}
