package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bilal/swarmgo/internal/memory"
)

// TestCoreMemoryReplaceAndAppend exercises both modes + both sections end-to-end
// through the tool surface and verifies the result via the store.
func TestCoreMemoryReplaceAndAppend(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	defer d.Close()
	mem := memory.New(d)
	const agentID = "agent-1"

	replace := NewCoreMemoryReplaceTool(mem, agentID)
	// No section → defaults to persona.
	if _, err := replace.Call(ctx, json.RawMessage(`{"content":"role: assistant"}`)); err != nil {
		t.Fatalf("replace persona: %v", err)
	}
	got, _ := mem.ReadCore(ctx, agentID, memory.CorePersona)
	if got != "role: assistant" {
		t.Fatalf("persona = %q after replace", got)
	}

	appendTool := NewCoreMemoryAppendTool(mem, agentID)
	// Explicit human section.
	if _, err := appendTool.Call(ctx, json.RawMessage(`{"content":"name: Bilal","section":"human"}`)); err != nil {
		t.Fatalf("append human: %v", err)
	}
	got, _ = mem.ReadCore(ctx, agentID, memory.CoreHuman)
	if got != "name: Bilal" {
		t.Fatalf("human = %q after append", got)
	}
	// persona untouched by the human append.
	got, _ = mem.ReadCore(ctx, agentID, memory.CorePersona)
	if got != "role: assistant" {
		t.Fatalf("persona changed: %q", got)
	}
}

// TestCoreMemoryRejectsEmpty ensures blank content is refused in both modes.
func TestCoreMemoryRejectsEmpty(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	defer d.Close()
	mem := memory.New(d)

	replace := NewCoreMemoryReplaceTool(mem, "agent-1")
	if _, err := replace.Call(ctx, json.RawMessage(`{"content":"   "}`)); err == nil {
		t.Fatalf("expected error for blank replace content")
	}
	appendTool := NewCoreMemoryAppendTool(mem, "agent-1")
	if _, err := appendTool.Call(ctx, json.RawMessage(`{"content":""}`)); err == nil {
		t.Fatalf("expected error for blank append content")
	}
}

// TestCoreMemoryNilStore returns a clear error when memory is unavailable.
func TestCoreMemoryNilStore(t *testing.T) {
	replace := NewCoreMemoryReplaceTool(nil, "agent-1")
	if _, err := replace.Call(context.Background(), json.RawMessage(`{"content":"x"}`)); err == nil {
		t.Fatalf("expected error when store is nil")
	}
}
