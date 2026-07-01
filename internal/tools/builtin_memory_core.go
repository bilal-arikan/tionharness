package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/db"
	"github.com/bilal-arikan/swarmgo/internal/memory"
	"github.com/bilal-arikan/swarmgo/internal/providers"
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

// labelSchema is the shared "label" property: which named core block the edit
// targets. Defaults to persona. The exact set is per-agent (persona + human by
// default, plus any custom blocks); an unknown label is rejected at call time
// with the list of valid labels, so the agent learns the available blocks.
const labelSchema = `"label":{"type":"string","description":"Which core-memory block to edit (default \"persona\"). Standard blocks: \"persona\" = facts about yourself (identity/behaviour), \"human\" = facts about the user (preferences/context). Your agent may define additional named blocks."}`

// coreMemoryDesc is the shared one-line explanation of core memory, factored out
// so core_memory_append and core_memory_replace don't each repeat it in full.
const coreMemoryDesc = "Core memory is small, persistent text re-injected into your context every turn (default blocks: \"persona\" = about you, \"human\" = about the user, plus any custom blocks)."

func (t CoreMemoryTool) Def() providers.ToolDef {
	if t.mode == "append" {
		return providers.ToolDef{
			Name:        "core_memory_append",
			Description: "Append one line to a named block of your core memory. " + coreMemoryDesc + " Use label=\"human\" for a durable fact about the user, \"persona\" for one about yourself. If a block is full, condense it with core_memory_replace.",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{
					"content":{"type":"string","description":"The line to append to the block"},
					` + labelSchema + `
				},
				"required":["content"],
				"additionalProperties":false
			}`),
		}
	}
	return providers.ToolDef{
		Name:        "core_memory_replace",
		Description: "Replace one named block of your core memory with new content. " + coreMemoryDesc + " Rewrite a block to keep it accurate and within its character limit.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"content":{"type":"string","description":"The new full content of the block"},
				` + labelSchema + `
			},
			"required":["content"],
			"additionalProperties":false
		}`),
	}
}

func (t CoreMemoryTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Content string `json:"content"`
		Label   string `json:"label"`
		Section string `json:"section"` // back-compat alias for label
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErr(err)
	}
	in.Content = strings.TrimSpace(in.Content)
	if in.Content == "" {
		return "", fmt.Errorf("content is required")
	}
	if t.mem == nil {
		return "", fmt.Errorf("memory is not available in this workspace")
	}
	// Resolve the target block. An empty label defaults to persona; "section" is
	// accepted as an alias so older prompts keep working.
	label := strings.ToLower(strings.TrimSpace(in.Label))
	if label == "" {
		label = strings.ToLower(strings.TrimSpace(in.Section))
	}
	if label == "" {
		label = memory.CorePersona
	}
	// Validate the label and read-only flag against the agent's defined blocks so
	// the agent gets an actionable error instead of silently writing nowhere.
	blocks, err := t.mem.CoreBlocks(ctx, t.agentID)
	if err != nil {
		return "", fmt.Errorf("load core blocks: %w", err)
	}
	var def *db.CoreBlock
	for i := range blocks {
		if strings.EqualFold(blocks[i].Label, label) {
			def = &blocks[i]
			break
		}
	}
	if def == nil {
		return "", fmt.Errorf("unknown core block %q; valid blocks: %s", label, strings.Join(blockLabels(blocks), ", "))
	}
	if def.ReadOnly {
		return "", fmt.Errorf("core block %q is read-only and cannot be edited by the agent", def.Label)
	}

	action := "core_replaced"
	if t.mode == "append" {
		action = "core_appended"
		err = t.mem.AppendCore(ctx, t.agentID, def.Label, in.Content)
	} else {
		err = t.mem.WriteCore(ctx, t.agentID, def.Label, in.Content)
	}
	if err != nil {
		// CoreBlockFullError already carries an actionable message (counts + advice).
		return "", err
	}
	b, _ := json.Marshal(map[string]string{"action": action, "label": def.Label})
	return string(b), nil
}

// blockLabels lists block labels for error messages.
func blockLabels(blocks []db.CoreBlock) []string {
	out := make([]string, len(blocks))
	for i, b := range blocks {
		out[i] = b.Label
	}
	return out
}
