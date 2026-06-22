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

// sectionSchema is the shared "section" property: which half of core memory the
// edit targets. persona = facts about yourself; human = facts about the user.
const sectionSchema = `"section":{"type":"string","enum":["persona","human"],"description":"Which core-memory section to edit: \"persona\" = facts about yourself (your identity/behaviour); \"human\" = facts about the user (their preferences/context). Defaults to persona."}`

func (t CoreMemoryTool) Def() providers.ToolDef {
	if t.mode == "append" {
		return providers.ToolDef{
			Name:        "core_memory_append",
			Description: "Append one line to a section of your core memory — the persistent working-memory block kept in context every turn. Use section=\"human\" to record a durable fact about the user, section=\"persona\" for a fact about yourself.",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{
					"content":{"type":"string","description":"The line to append to the section"},
					` + sectionSchema + `
				},
				"required":["content"],
				"additionalProperties":false
			}`),
		}
	}
	return providers.ToolDef{
		Name:        "core_memory_replace",
		Description: "Replace one section of your core memory with new content. Core memory is a small, persistent block kept in context every turn, split into \"persona\" (about you) and \"human\" (about the user); rewrite a section to keep it accurate and concise.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"content":{"type":"string","description":"The new full content of the section"},
				` + sectionSchema + `
			},
			"required":["content"],
			"additionalProperties":false
		}`),
	}
}

func (t CoreMemoryTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Content string `json:"content"`
		Section string `json:"section"`
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
	// Normalize the section; an empty/unknown value falls back to persona in the
	// store. Echo the resolved section back so the caller sees where it landed.
	section := memory.CorePersona
	if strings.EqualFold(strings.TrimSpace(in.Section), memory.CoreHuman) {
		section = memory.CoreHuman
	}
	if t.mode == "append" {
		if err := t.mem.AppendCore(ctx, t.agentID, section, in.Content); err != nil {
			return "", fmt.Errorf("append core memory: %w", err)
		}
		b, _ := json.Marshal(map[string]string{"action": "core_appended", "section": section})
		return string(b), nil
	}
	if err := t.mem.WriteCore(ctx, t.agentID, section, in.Content); err != nil {
		return "", fmt.Errorf("replace core memory: %w", err)
	}
	b, _ := json.Marshal(map[string]string{"action": "core_replaced", "section": section})
	return string(b), nil
}
