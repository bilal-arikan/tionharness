package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal/swarmgo/internal/memory"
	"github.com/bilal/swarmgo/internal/providers"
)

// MemoryRecallTool lets an agent search its own long-term memory on demand,
// complementing the automatic recall already injected into the system prompt.
type MemoryRecallTool struct {
	store   *memory.Store
	agentID string
}

// NewMemoryRecallTool binds the tool to one agent's memory.
func NewMemoryRecallTool(store *memory.Store, agentID string) MemoryRecallTool {
	return MemoryRecallTool{store: store, agentID: agentID}
}

func (MemoryRecallTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "memory_recall",
		Description: "Search your long-term memory for entries relevant to a query. Returns the top matches with scores.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"query":{"type":"string","description":"What to search your memory for"},
				"limit":{"type":"integer","description":"Max results (default 5)"}
			},
			"required":["query"],
			"additionalProperties":false
		}`),
	}
}

func (t MemoryRecallTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if strings.TrimSpace(args.Query) == "" {
		return "", fmt.Errorf("query is required")
	}
	hits, err := t.store.Recall(ctx, t.agentID, args.Query, args.Limit)
	if err != nil {
		return "", err
	}
	if len(hits) == 0 {
		return "No relevant memories found.", nil
	}
	var b strings.Builder
	for i, h := range hits {
		fmt.Fprintf(&b, "%d. [%s, score %.3f] %s\n", i+1, h.Source.Kind, h.Score, h.Source.Content)
	}
	return strings.TrimSpace(b.String()), nil
}
