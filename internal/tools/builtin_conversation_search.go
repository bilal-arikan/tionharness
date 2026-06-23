package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bilal-arikan/swarmgo/internal/db"
	"github.com/bilal-arikan/swarmgo/internal/providers"
)

// ConversationSearchTool lets an agent full-text search the message history of
// the workspace's sessions — the deep "what was actually said" complement to
// list_sessions (which only sees titles + summaries). Backed by db.SearchMessages,
// a pure in-memory scan, so it adds no LLM call and no external dependency.
type ConversationSearchTool struct{ db *db.DB }

// NewConversationSearchTool binds the tool to a workspace DB.
func NewConversationSearchTool(database *db.DB) ConversationSearchTool {
	return ConversationSearchTool{db: database}
}

func (ConversationSearchTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "conversation_search",
		Description: "Full-text search the message history of this workspace's chat sessions for a " +
			"keyword or phrase. Returns matching turns with a snippet, session title and age — use it " +
			"to recall what was said or decided in past conversations (deeper than list_sessions, which " +
			"only sees titles and summaries). Multiple words are AND-matched.\n\n" +
			"Verbatim recovery: after a long chat is compacted, early turns survive only as a summary. " +
			"Use full=true to get the WHOLE matched message word-for-word (not just a snippet), and " +
			"context=N to also include the N turns before and after each hit so you see the surrounding " +
			"exchange. Pass session_id to search only one session (e.g. the current one).",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "query": { "type": "string", "description": "Keyword(s) or phrase to find. Multiple words must all appear in a message." },
    "limit": { "type": "integer", "description": "Max results (default 15)." },
    "role": { "type": "string", "enum": ["user", "assistant", "all"], "description": "Restrict to a sender role (default all)." },
    "session_id": { "type": "string", "description": "Restrict the search to a single session id (default: all sessions in the workspace)." },
    "full": { "type": "boolean", "description": "Return the full text of each matched message verbatim instead of a ~160-char snippet (default false)." },
    "context": { "type": "integer", "description": "Also include this many turns immediately before AND after each hit, verbatim, for surrounding context (0-5, default 0)." }
  },
  "required": ["query"],
  "additionalProperties": false
}`),
		Examples: []json.RawMessage{
			json.RawMessage(`{"query":"first question","session_id":"SES42","full":true,"context":1}`),
		},
	}
}

func (t ConversationSearchTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Query     string `json:"query"`
		Limit     int    `json:"limit"`
		Role      string `json:"role"`
		SessionID string `json:"session_id"`
		Full      bool   `json:"full"`
		Context   int    `json:"context"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", argErr(err)
	}
	if strings.TrimSpace(args.Query) == "" {
		return "", fmt.Errorf("query is required")
	}
	if args.Limit <= 0 {
		args.Limit = 15
	}
	if args.Context < 0 {
		args.Context = 0
	}
	if args.Context > 5 {
		args.Context = 5
	}
	opts := db.SearchOpts{Query: args.Query, Limit: args.Limit, OnlyID: strings.TrimSpace(args.SessionID)}
	if args.Role != "" && args.Role != "all" {
		opts.Roles = []string{args.Role}
	}

	hits, err := t.db.SearchMessages(ctx, opts)
	if err != nil {
		return "", err
	}
	if len(hits) == 0 {
		return "No matching messages in this workspace.", nil
	}

	now := time.Now().Unix()
	var b strings.Builder
	for _, h := range hits {
		title := h.SessionTitle
		if title == "" {
			title = "(untitled)"
		}
		// Header: role, session title, session id (so the agent can re-scope), age.
		fmt.Fprintf(&b, "- [%s] %q · %s · %s\n", h.Role, sessClip(title, 60), h.SessionID, sessAge(now-h.CreatedAt))

		switch {
		case args.Context > 0:
			// Verbatim surrounding turns (chronological); the hit is marked »».
			around := t.db.MessagesAround(ctx, h.SessionID, h.MessageID, args.Context, args.Context)
			for _, m := range around {
				marker := "  "
				if m.ID == h.MessageID {
					marker = "»»"
				}
				fmt.Fprintf(&b, "  %s %s: %s\n", marker, m.Role, clipText(m.Text, args.Full))
			}
		case args.Full:
			// Full matched message verbatim — the snippet alone is truncated.
			text := h.Snippet
			if around := t.db.MessagesAround(ctx, h.SessionID, h.MessageID, 0, 0); len(around) == 1 {
				text = strings.TrimSpace(around[0].Text)
			}
			fmt.Fprintf(&b, "    %s\n", text)
		default:
			fmt.Fprintf(&b, "    %s\n", h.Snippet)
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String()), nil
}

// clipText collapses whitespace and returns the full text when full is set, else
// a one-line ~160-char clip for the context view.
func clipText(text string, full bool) string {
	if full {
		return strings.TrimSpace(text)
	}
	return sessClip(strings.Join(strings.Fields(text), " "), 160)
}
