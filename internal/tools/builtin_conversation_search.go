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
			"only sees titles and summaries). Multiple words are AND-matched.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "query": { "type": "string", "description": "Keyword(s) or phrase to find. Multiple words must all appear in a message." },
    "limit": { "type": "integer", "description": "Max results (default 15)." },
    "role": { "type": "string", "enum": ["user", "assistant", "all"], "description": "Restrict to a sender role (default all)." }
  },
  "required": ["query"],
  "additionalProperties": false
}`),
	}
}

func (t ConversationSearchTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
		Role  string `json:"role"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if strings.TrimSpace(args.Query) == "" {
		return "", fmt.Errorf("query is required")
	}
	if args.Limit <= 0 {
		args.Limit = 15
	}
	opts := db.SearchOpts{Query: args.Query, Limit: args.Limit}
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
		fmt.Fprintf(&b, "- [%s] %q · %s — %s\n", h.Role, sessClip(title, 60), sessAge(now-h.CreatedAt), h.Snippet)
	}
	return strings.TrimSpace(b.String()), nil
}
