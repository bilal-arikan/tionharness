package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

// focusViewInput is the argument shape for the focus_view tool.
type focusViewInput struct {
	View      string `json:"view"`
	SessionID string `json:"sessionId"`
	AgentID   string `json:"agentId"`
}

// focusableViews are the NavRail views focus_view can drive the UI to. Kept in
// sync with the frontend View union (lib/url.ts VIEWS); an unknown view is
// rejected so a typo can't emit a navigation the UI silently ignores.
var focusableViews = map[string]bool{
	"chat": true, "executions": true, "agents": true, "explorer": true,
	"board": true, "schedules": true, "memory": true, "flows": true,
	"artifacts": true, "skills": true, "market": true, "budget": true,
	"prompts": true, "workspace": true, "settings": true,
}

// FocusViewTool lets an agent drive the user's UI to a specific view (and
// entity), so it can direct attention — "open the artifact I just made", "look
// at the board". Non-blocking: it fires a navigation event open windows apply at
// once and returns. When no navigate sink is wired (autonomous run with no open
// client), it is a graceful no-op so the turn never breaks. Both provider paths
// share this one definition (single-schema rule).
type FocusViewTool struct{}

// NewFocusViewTool constructs the focus_view tool.
func NewFocusViewTool() FocusViewTool { return FocusViewTool{} }

func (FocusViewTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "focus_view",
		Description: "Drive the user's UI to a screen to direct their attention (e.g. open the " +
			"artifacts/board/flows/prompts view, or jump to a chat session). Does NOT block — it navigates " +
			"open windows and returns. For 'chat'/'executions' the optional sessionId selects a " +
			"session (defaults to THIS session); for 'agents'/'memory' agentId selects an agent " +
			"(defaults to the responding agent). Use to show, not to ask.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "view": { "type": "string", "enum": ["chat","executions","agents","explorer","board","schedules","memory","flows","artifacts","skills","market","budget","prompts","workspace","settings"], "description": "The screen to open." },
    "sessionId": { "type": "string", "description": "Optional session to select (chat/executions views). Defaults to the current session." },
    "agentId": { "type": "string", "description": "Optional agent to select (agents/memory views). Defaults to the responding agent." }
  },
  "required": ["view"],
  "additionalProperties": false
}`),
	}
}

func (FocusViewTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	in, err := parseInput[focusViewInput]("focus_view", input)
	if err != nil {
		return "", err
	}
	view := strings.TrimSpace(in.View)
	if view == "" {
		return "", fmt.Errorf("view is required")
	}
	if !focusableViews[view] {
		return "", fmt.Errorf("unknown view %q", view)
	}
	sink := navigateFrom(ctx)
	if sink == nil {
		// No client is listening (autonomous run / no open window): don't fail the
		// turn — report that the navigation couldn't surface so the model proceeds.
		return "no UI is available to navigate for this turn (no open client); nothing changed", nil
	}
	if err := sink.Navigate(ctx, NavigateSpec{View: view, SessionID: in.SessionID, AgentID: in.AgentID}); err != nil {
		return "", err
	}
	return "navigated the UI to " + view, nil
}
