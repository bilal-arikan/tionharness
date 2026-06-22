package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/providers"
)

// Hook self-management tools let an agent read and manage PreToolUse/PostToolUse
// hooks in its workspace — external commands that intercept native tool calls
// (Claude Code hook contract). Safety boundary mirrors the rest of the
// self-management surface: list/create are unrestricted, delete is limited to
// hooks the agent itself created (provenance via Hook.CreatedBy).

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
		Name:        "list_hooks",
		Description: "List the PreToolUse/PostToolUse hooks in this workspace. Each hook runs an external command around a native tool call (matched by a tool-name glob; empty = all tools). Returns id, event, matcher, command, enabled, and whether you created it (and may therefore delete it).",
		InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
	}
}

func (t ListHooksTool) Call(ctx context.Context, _ json.RawMessage) (string, error) {
	hooks, err := t.d.db.ListHooks(ctx)
	if err != nil {
		return "", err
	}
	type row struct {
		ID             string `json:"id"`
		Event          string `json:"event"`
		Matcher        string `json:"matcher"`
		Command        string `json:"command"`
		Enabled        bool   `json:"enabled"`
		CreatedByAgent bool   `json:"createdByAgent"`
	}
	out := make([]row, 0, len(hooks))
	for _, h := range hooks {
		out = append(out, row{
			ID: h.ID, Event: h.Event, Matcher: h.Matcher,
			Command: truncateForTool(h.Command, 200), Enabled: h.Enabled,
			CreatedByAgent: h.CreatedBy != "",
		})
	}
	b, _ := json.Marshal(out)
	return string(b), nil
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
		Description: "Create a PreToolUse or PostToolUse hook: an external command run around matching native tool calls. The command speaks the Claude Code hook contract (JSON on stdin, JSON decision on stdout). event is PreToolUse or PostToolUse; matcher is a tool-name glob (empty = all tools). The hook is enabled and tagged as created by you. Returns the new hook id.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"event":{"type":"string","enum":["PreToolUse","PostToolUse"]},
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
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if in.Event != db.HookPreToolUse && in.Event != db.HookPostToolUse {
		return "", fmt.Errorf("event must be PreToolUse or PostToolUse")
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

// ---- delete_hook ----

// DeleteHookTool removes an agent-created hook (provenance-enforced).
type DeleteHookTool struct{ d hookDeps }

// NewDeleteHookTool constructs delete_hook.
func NewDeleteHookTool(database *db.DB, actorID string) DeleteHookTool {
	return DeleteHookTool{d: hookDeps{db: database, actorID: actorID}}
}

func (DeleteHookTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "delete_hook",
		Description: "Delete an agent-created hook (not one made by the user). Pass the hook id.",
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
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	in.ID = strings.TrimSpace(in.ID)
	if in.ID == "" {
		return "", fmt.Errorf("id is required")
	}
	cur, err := t.d.db.GetHook(ctx, in.ID)
	if err != nil {
		return "", fmt.Errorf("no hook with id %q (use list_hooks)", in.ID)
	}
	if cur.CreatedBy == "" {
		return "", fmt.Errorf("hook %q was created by the user and cannot be deleted by an agent", in.ID)
	}
	if err := t.d.db.DeleteHook(ctx, in.ID); err != nil {
		return "", fmt.Errorf("delete hook: %w", err)
	}
	b, _ := json.Marshal(map[string]string{"id": in.ID, "action": "deleted"})
	return string(b), nil
}
