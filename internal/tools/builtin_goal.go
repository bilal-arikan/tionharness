package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/providers"
)

// maxSessionGoalLen bounds a goal so it can never blow up the context window
// (it is re-injected into every turn). Mirrors the HTTP handler's limit.
const maxSessionGoalLen = 2000

// SetSessionGoalTool sets this session's persistent objective — the SAME "north
// star" the user can set in the UI (db.Session.Goal). Once set it is injected
// into every turn's context so the agent keeps replies aligned with it. Shared,
// not parallel: setting it overwrites whatever goal the session currently holds
// (the response flags a replacement so it stays transparent). No-op without a
// goal sink (autonomous run with no session). Non-blocking.
type SetSessionGoalTool struct{}

// NewSetSessionGoalTool constructs the set_session_goal tool.
func NewSetSessionGoalTool() SetSessionGoalTool { return SetSessionGoalTool{} }

func (SetSessionGoalTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "set_session_goal",
		Description: "Set this session's persistent objective (its 'north star'). Once set it is " +
			"injected into every turn so you keep replies aligned with it until it is met. This is " +
			"the SAME goal the user sets in the UI — setting it here overwrites the current one. Use " +
			"for a single durable objective, not a task checklist (use todo_write for that).",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "goal": { "type": "string", "description": "The objective, phrased as a short measurable north star (max 2000 chars)." }
  },
  "required": ["goal"],
  "additionalProperties": false
}`),
	}
}

func (SetSessionGoalTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Goal string `json:"goal"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErrFor("set_session_goal", err)
	}
	goal := strings.TrimSpace(in.Goal)
	if goal == "" {
		return "", fmt.Errorf("goal is required")
	}
	if len([]rune(goal)) > maxSessionGoalLen {
		return "", fmt.Errorf("goal too long (max %d chars)", maxSessionGoalLen)
	}
	sink := goalFrom(ctx)
	if sink == nil {
		return "no session is available to set a goal for this turn", nil
	}
	// Read the current goal first so we can flag a replacement (transparency: the
	// goal is shared with the user, so silently clobbering theirs would surprise).
	prev, _ := sink.Goal(ctx)
	if err := sink.SetGoal(ctx, goal, false); err != nil {
		return "", err
	}
	msg := "session goal set: " + goal
	if p := strings.TrimSpace(prev.Text); p != "" && p != goal {
		msg += "\n(replaced the previous goal: " + goalClip(p) + ")"
	}
	return msg, nil
}

// CompleteGoalTool marks this session's goal as achieved: it stays stored (so the
// user can review or reopen it) but stops being injected into context — a
// completed objective should no longer steer the conversation. No-op without a
// goal sink. Non-blocking.
type CompleteGoalTool struct{}

// NewCompleteGoalTool constructs the complete_goal tool.
func NewCompleteGoalTool() CompleteGoalTool { return CompleteGoalTool{} }

func (CompleteGoalTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "complete_goal",
		Description: "Mark this session's goal as achieved. It stays visible but stops steering " +
			"future turns (its context injection stops). Call this once the north-star objective is " +
			"genuinely met. Takes no arguments.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
	}
}

func (CompleteGoalTool) Call(ctx context.Context, _ json.RawMessage) (string, error) {
	sink := goalFrom(ctx)
	if sink == nil {
		return "no session is available to complete a goal for this turn", nil
	}
	cur, err := sink.Goal(ctx)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(cur.Text) == "" {
		return "no goal is set on this session; nothing to complete", nil
	}
	if cur.Done {
		return "the goal is already marked complete", nil
	}
	if err := sink.SetGoal(ctx, cur.Text, true); err != nil {
		return "", err
	}
	return "goal marked complete: " + cur.Text + "\n(it stays visible but no longer steers future turns)", nil
}

// goalClip caps a goal echo so a long replaced goal can't bloat the result.
func goalClip(s string) string {
	r := []rune(s)
	if len(r) <= 80 {
		return s
	}
	return strings.TrimSpace(string(r[:80])) + "…"
}
