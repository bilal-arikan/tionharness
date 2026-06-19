package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/memory"
	"github.com/bilal/swarmgo/internal/providers"
)

// MemoryAddTool lets an agent write a long-term memory for itself — a fact,
// note or summary it wants to recall later (complements memory_recall, which
// reads). The memory is stored under the acting agent's id, so each agent owns
// its own memories.
type MemoryAddTool struct {
	mem     *memory.Store
	agentID string
}

// NewMemoryAddTool binds memory_add to the workspace memory store for one agent.
func NewMemoryAddTool(mem *memory.Store, agentID string) MemoryAddTool {
	return MemoryAddTool{mem: mem, agentID: agentID}
}

func (MemoryAddTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "memory_add",
		Description: "Save a long-term memory for yourself so you can recall it in future conversations (use memory_recall to retrieve). Use kind=document for facts/notes the user gave you, kind=reflection for your own summaries/insights.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"content":{"type":"string","description":"The fact, note or insight to remember"},
				"kind":{"type":"string","enum":["document","reflection"],"description":"document = user-provided fact/note (default); reflection = your own summary/insight"}
			},
			"required":["content"],
			"additionalProperties":false
		}`),
	}
}

func (t MemoryAddTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Content string `json:"content"`
		Kind    string `json:"kind"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	in.Content = strings.TrimSpace(in.Content)
	if in.Content == "" {
		return "", fmt.Errorf("content is required")
	}
	kind := strings.TrimSpace(strings.ToLower(in.Kind))
	switch kind {
	case "", "document":
		kind = db.MemoryDocument
	case "reflection":
		kind = db.MemoryReflection
	default:
		return "", fmt.Errorf("invalid kind %q (want document or reflection)", in.Kind)
	}
	if t.mem == nil {
		return "", fmt.Errorf("memory is not available in this workspace")
	}
	src, err := t.mem.Remember(ctx, t.agentID, kind, in.Content)
	if err != nil {
		return "", fmt.Errorf("save memory: %w", err)
	}
	b, _ := json.Marshal(map[string]string{"id": src.ID, "kind": kind, "action": "saved"})
	return string(b), nil
}
