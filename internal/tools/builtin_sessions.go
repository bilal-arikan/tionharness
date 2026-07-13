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

// ListSessionsTool lets an agent pull the workspace's sessions on demand —
// the "pull" complement to the cross-session context block. That block pushes
// only RECENT (past) chat sessions into the prompt; ACTIVE (live) sessions and
// autonomous runs (spawn/worker, flow, task, schedule) are never auto-injected,
// so this tool is the agent's only way to see them. It lists every kind by
// default (an optional kind filter narrows it), reuses each session's stored
// Title and rolling Summary (no new LLM call) and is scoped to this workspace's DB.
type ListSessionsTool struct{ db *db.DB }

// NewListSessionsTool binds the tool to a workspace DB.
func NewListSessionsTool(database *db.DB) ListSessionsTool {
	return ListSessionsTool{db: database}
}

// listSessionsPageLimit is the default page size when the caller doesn't pass a
// limit. ALL sessions are reachable by paging with the offset argument.
const listSessionsPageLimit = 20

func (ListSessionsTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "list_sessions",
		Description: "List the sessions in this workspace for situational awareness — their kind, " +
			"title, message counts, age and a short summary. Covers EVERY execution path by default: " +
			"chat threads plus autonomous runs (spawn/worker, flow, task, schedule). Active (live) " +
			"sessions are NOT auto-injected into your context, so call this tool whenever you need to " +
			"see what other work is currently in progress. Returns active sessions of all kinds by " +
			"default; pass kind:\"...\" to narrow to one kind and state:\"all\" to include past " +
			"(archived) ones. Each line is prefixed with [kind·state]. Results are paginated (newest " +
			"first): the reply reports the total and, when more remain, the exact offset for the next page.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "state": { "type": "string", "enum": ["active", "all"], "description": "Which sessions to list (default: active)." },
    "kind": { "type": "string", "enum": ["chat", "spawned", "worker", "flow", "task", "schedule"], "description": "Narrow to a single session kind. Omit to list all kinds (default)." },
    "limit": { "type": "integer", "description": "Max sessions per page (default 20). Use with offset to page through all of them." },
    "offset": { "type": "integer", "description": "How many matching sessions to skip before this page (default 0). Pass the offset from a previous reply to get the next page." }
  },
  "additionalProperties": false
}`),
	}
}

func (t ListSessionsTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		State  string `json:"state"`
		Kind   string `json:"kind"`
		Limit  int    `json:"limit"`
		Offset int    `json:"offset"`
	}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &args); err != nil {
			return "", argErr(err)
		}
	}
	if args.Limit <= 0 {
		args.Limit = listSessionsPageLimit
	}
	if args.Offset < 0 {
		args.Offset = 0
	}
	onlyActive := args.State != "all"
	kindFilter := strings.TrimSpace(args.Kind)

	sessions, err := t.db.ListSessions(ctx, "") // workspace-wide, UpdatedAt desc
	if err != nil {
		return "", err
	}

	// Collect every matching session first so we know the true total, then window
	// it by [offset, offset+limit) for pagination. All kinds are listed by default
	// (chat + autonomous runs); an optional kind filter narrows to one. Legacy
	// sessions persisted before the kind field carry "" and count as chat.
	matches := make([]db.Session, 0, len(sessions))
	for _, s := range sessions {
		if kindFilter != "" && !sessKindMatches(s.Kind, kindFilter) {
			continue
		}
		if onlyActive && s.State != "active" {
			continue
		}
		matches = append(matches, s)
	}
	total := len(matches)
	if total == 0 {
		return "No matching sessions in this workspace.", nil
	}
	if args.Offset >= total {
		return fmt.Sprintf("Offset %d is past the last of %d matching sessions.", args.Offset, total), nil
	}

	end := args.Offset + args.Limit
	if end > total {
		end = total
	}
	page := matches[args.Offset:end]

	now := time.Now().Unix()
	var b strings.Builder
	for _, s := range page {
		title := strings.TrimSpace(s.Title)
		if title == "" {
			title = "(untitled)"
		}
		fmt.Fprintf(&b, "- [%s·%s] %q · %d msg · %s", sessKindLabel(s.Kind), s.State, sessClip(title, 70), s.MessageCount, sessAge(now-s.UpdatedAt))
		if snip := sessSnippet(s.Summary); snip != "" {
			b.WriteString(" — " + snip)
		}
		b.WriteString("\n")
	}

	fmt.Fprintf(&b, "\nShowing %d–%d of %d.", args.Offset+1, end, total)
	if end < total {
		fmt.Fprintf(&b, " %d more — pass offset:%d for the next page.", total-end, end)
	}
	return strings.TrimSpace(b.String()), nil
}

// sessKindMatches reports whether a session's kind belongs under a kind filter.
// A legacy session persisted before the kind field existed carries "" and counts
// as a plain chat, so it stays reachable under the "chat" filter.
func sessKindMatches(kind, filter string) bool {
	if filter == "chat" {
		return kind == "" || kind == "chat"
	}
	return kind == filter
}

// sessKindLabel normalizes a session kind for the listing prefix, mapping the
// empty legacy kind to "chat".
func sessKindLabel(kind string) string {
	if kind == "" {
		return "chat"
	}
	return kind
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
