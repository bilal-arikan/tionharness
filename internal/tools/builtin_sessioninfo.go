package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// GetSessionInfoTool returns the metadata of the session the agent is running
// in (or, with an explicit id, another session in this workspace): title, state,
// kind, bound agent, tags, goal, working directory, lineage (parent/handoff) and
// the coordinator/worker role. It is the read counterpart of the session-edit
// tools (set_session_title / set_session_tags / set_session_goal / ...), so the
// agent can inspect before it mutates — and self-orient in a fresh autonomous
// turn without asking the user.
type GetSessionInfoTool struct{ db *db.DB }

// NewGetSessionInfoTool binds the tool to a workspace DB.
func NewGetSessionInfoTool(database *db.DB) GetSessionInfoTool {
	return GetSessionInfoTool{db: database}
}

func (GetSessionInfoTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "get_session_info",
		Description: "Read this session's metadata: id, title, state, kind, bound agent, tags, " +
			"goal, working directory, coordinator/worker role and lineage (parent / handoff). " +
			"Use it to orient yourself before editing the session (set_session_title / " +
			"set_session_tags / set_session_goal) or to check what a tag-triggered automation " +
			"will see. Pass session_id to inspect another session in this workspace instead.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "session_id": { "type": "string", "description": "Session to inspect (default: the session this turn runs in)." }
  },
  "additionalProperties": false
}`),
	}
}

func (t GetSessionInfoTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		SessionID string `json:"session_id"`
	}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &args); err != nil {
			return "", argErrFor("get_session_info", err)
		}
	}
	sid := strings.TrimSpace(args.SessionID)
	if sid == "" {
		sid = CurrentSessionID(ctx)
	}
	if sid == "" {
		return "no session is bound to this turn (pass session_id to inspect one explicitly)", nil
	}
	s, err := t.db.GetSession(ctx, sid)
	if err != nil {
		return "", fmt.Errorf("session %s not found in this workspace: %w", sid, err)
	}

	var b strings.Builder
	line := func(label, val string) {
		if val = strings.TrimSpace(val); val != "" {
			fmt.Fprintf(&b, "%s: %s\n", label, val)
		}
	}
	line("id", s.ID)
	line("title", s.Title)
	line("state", s.State)
	line("kind", s.Kind)
	agentLabel := s.AgentID
	if a, err := t.db.GetAgent(ctx, s.AgentID); err == nil && strings.TrimSpace(a.Name) != "" {
		agentLabel = fmt.Sprintf("%s (%s)", a.Name, s.AgentID)
	}
	line("agent", agentLabel)
	fmt.Fprintf(&b, "messages: %d\n", s.MessageCount)
	if len(s.Tags) > 0 {
		line("tags", strings.Join(s.Tags, ", "))
	}
	if s.Goal != "" {
		goal := s.Goal
		if s.GoalDone {
			goal += " (done)"
		}
		line("goal", goal)
	}
	line("working_dir", s.WorkingDir)
	line("role", s.Role)
	line("coordinator_session", s.CoordinatorSessionID)
	line("parent_session", s.ParentSessionID)
	return strings.TrimRight(b.String(), "\n"), nil
}
