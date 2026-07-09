package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// ListSessionsTool lets an agent pull the workspace's chat sessions on demand —
// the "pull" complement to the cross-session context block. That block pushes
// only RECENT (past) sessions into the prompt; ACTIVE (live) sessions are never
// auto-injected, so this tool is the agent's only way to see them. It reuses
// each session's stored Title and rolling Summary (no new LLM call) and is
// scoped to this workspace's DB.
type ListSessionsTool struct{ db *db.DB }

// NewListSessionsTool binds the tool to a workspace DB.
func NewListSessionsTool(database *db.DB) ListSessionsTool {
	return ListSessionsTool{db: database}
}

func (ListSessionsTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "list_sessions",
		Description: "List the chat sessions in this workspace for situational awareness — " +
			"their titles, message counts, age and a short summary. Active (live) sessions are " +
			"NOT auto-injected into your context, so call this tool whenever you need to see what " +
			"other work is currently in progress. Returns active sessions by default; pass " +
			"state:\"all\" to include past ones.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "state": { "type": "string", "enum": ["active", "all"], "description": "Which sessions to list (default: active)." },
    "limit": { "type": "integer", "description": "Max sessions to return (default 15)." }
  },
  "additionalProperties": false
}`),
	}
}

func (t ListSessionsTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		State string `json:"state"`
		Limit int    `json:"limit"`
	}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &args); err != nil {
			return "", argErr(err)
		}
	}
	if args.Limit <= 0 {
		args.Limit = 15
	}
	onlyActive := args.State != "all"

	sessions, err := t.db.ListSessions(ctx, "") // workspace-wide, UpdatedAt desc
	if err != nil {
		return "", err
	}

	now := time.Now().Unix()
	var b strings.Builder
	n := 0
	for _, s := range sessions {
		if s.Kind != "chat" {
			continue
		}
		if onlyActive && s.State != "active" {
			continue
		}
		if n >= args.Limit {
			break
		}
		title := strings.TrimSpace(s.Title)
		if title == "" {
			title = "(untitled)"
		}
		fmt.Fprintf(&b, "- [%s] %q · %d msg · %s", s.State, sessClip(title, 70), s.MessageCount, sessAge(now-s.UpdatedAt))
		if snip := sessSnippet(s.Summary); snip != "" {
			b.WriteString(" — " + snip)
		}
		b.WriteString("\n")
		n++
	}
	if n == 0 {
		return "No matching sessions in this workspace.", nil
	}
	return strings.TrimSpace(b.String()), nil
}

// sessSnippet returns the first non-empty line of a summary, capped.
func sessSnippet(summary string) string {
	for _, ln := range strings.Split(summary, "\n") {
		if ln = strings.TrimSpace(ln); ln != "" {
			return sessClip(ln, 140)
		}
	}
	return ""
}

func sessClip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n])) + "…"
}

func sessAge(sec int64) string {
	switch {
	case sec < 60:
		return "just now"
	case sec < 3600:
		return fmt.Sprintf("%dm ago", sec/60)
	case sec < 86400:
		return fmt.Sprintf("%dh ago", sec/3600)
	default:
		return fmt.Sprintf("%dd ago", sec/86400)
	}
}
