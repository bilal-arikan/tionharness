package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal/swarmgo/internal/memory"
	"github.com/bilal/swarmgo/internal/providers"
)

// CoreMemoryTool lets an agent edit its single, persistent "core" working-memory
// block — the MemGPT/Letta idea of memory the agent maintains itself and that is
// re-injected into every prompt. Two modes share one type: "replace" overwrites
// the whole block, "append" adds a line. The block is stored under the acting
// agent's id (one core row per agent), mirroring MemoryAddTool's ownership model.
type CoreMemoryTool struct {
	mem     *memory.Store
	agentID string
	mode    string // "replace" | "append"
}

// NewCoreMemoryReplaceTool binds core_memory_replace to one agent's core block.
func NewCoreMemoryReplaceTool(mem *memory.Store, agentID string) CoreMemoryTool {
	return CoreMemoryTool{mem: mem, agentID: agentID, mode: "replace"}
}

// NewCoreMemoryAppendTool binds core_memory_append to one agent's core block.
func NewCoreMemoryAppendTool(mem *memory.Store, agentID string) CoreMemoryTool {
	return CoreMemoryTool{mem: mem, agentID: agentID, mode: "append"}
}

func (t CoreMemoryTool) Def() providers.ToolDef {
	if t.mode == "append" {
		return providers.ToolDef{
			Name:        "core_memory_append",
			Description: "Append one line to your core memory — the persistent working-memory block kept in context every turn. Use it to record a new durable fact about yourself or the user without rewriting the whole block.",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{
					"content":{"type":"string","description":"The line to append to your core memory"}
				},
				"required":["content"],
				"additionalProperties":false
			}`),
		}
	}
	return providers.ToolDef{
		Name:        "core_memory_replace",
		Description: "Replace your entire core memory block with new content. Core memory is a small, persistent block kept in context every turn; rewrite it to keep it accurate and concise (e.g. update a fact that changed).",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"content":{"type":"string","description":"The new full content of your core memory block"}
			},
			"required":["content"],
			"additionalProperties":false
		}`),
	}
}

func (t CoreMemoryTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	in.Content = strings.TrimSpace(in.Content)
	if in.Content == "" {
		return "", fmt.Errorf("content is required")
	}
	if t.mem == nil {
		return "", fmt.Errorf("memory is not available in this workspace")
	}
	if t.mode == "append" {
		if err := t.mem.AppendCore(ctx, t.agentID, in.Content); err != nil {
			return "", fmt.Errorf("append core memory: %w", err)
		}
		b, _ := json.Marshal(map[string]string{"action": "core_appended"})
		return string(b), nil
	}
	if err := t.mem.WriteCore(ctx, t.agentID, in.Content); err != nil {
		return "", fmt.Errorf("replace core memory: %w", err)
	}
	b, _ := json.Marshal(map[string]string{"action": "core_replaced"})
	return string(b), nil
}
