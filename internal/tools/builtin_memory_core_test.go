package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bilal/swarmgo/internal/memory"
)

// TestCoreMemoryReplaceAndAppend exercises both modes end-to-end through the
// tool surface and verifies the resulting block via the store.
func TestCoreMemoryReplaceAndAppend(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	defer d.Close()
	mem := memory.New(d)
	const agentID = "agent-1"

	replace := NewCoreMemoryReplaceTool(mem, agentID)
	if _, err := replace.Call(ctx, json.RawMessage(`{"content":"name: Bilal"}`)); err != nil {
		t.Fatalf("replace: %v", err)
	}
	got, _ := mem.ReadCore(ctx, agentID)
	if got != "name: Bilal" {
		t.Fatalf("core = %q after replace", got)
	}

	appendTool := NewCoreMemoryAppendTool(mem, agentID)
	if _, err := appendTool.Call(ctx, json.RawMessage(`{"content":"role: engineer"}`)); err != nil {
		t.Fatalf("append: %v", err)
	}
	got, _ = mem.ReadCore(ctx, agentID)
	if got != "name: Bilal\nrole: engineer" {
		t.Fatalf("core = %q after append", got)
	}

	// Replace overwrites the whole block.
	if _, err := replace.Call(ctx, json.RawMessage(`{"content":"fresh"}`)); err != nil {
		t.Fatalf("replace 2: %v", err)
	}
	got, _ = mem.ReadCore(ctx, agentID)
	if got != "fresh" {
		t.Fatalf("core = %q after second replace", got)
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
