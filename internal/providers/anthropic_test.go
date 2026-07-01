package providers

import (
	"encoding/json"
	"testing"
)

// asBlocks marshals a systemField result back to []systemBlock for assertions.
func asBlocks(t *testing.T, v any) []systemBlock {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var blocks []systemBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		t.Fatalf("unmarshal blocks: %v", err)
	}
	return blocks
}

func TestSystemField_NoCacheConcatenates(t *testing.T) {
	a := &Anthropic{} // extendedCache off
	got := a.systemField("PERSONA", "MEMORY")
	s, ok := got.(string)
	if !ok {
		t.Fatalf("want plain string without caching, got %T", got)
	}
	if s != "PERSONA\n\nMEMORY" {
		t.Errorf("system = %q, want %q", s, "PERSONA\n\nMEMORY")
	}
}

func TestSystemField_EmptyReturnsNil(t *testing.T) {
	a := &Anthropic{extendedCache: true}
	if got := a.systemField("  ", ""); got != nil {
		t.Errorf("empty system = %v, want nil", got)
	}
}

func TestSystemField_CacheBreakpointOnStaticOnly(t *testing.T) {
	a := &Anthropic{extendedCache: true}
	blocks := asBlocks(t, a.systemField("PERSONA", "MEMORY"))
	if len(blocks) != 2 {
		t.Fatalf("got %d blocks, want 2", len(blocks))
	}
	if blocks[0].Text != "PERSONA" || blocks[0].CacheControl == nil {
		t.Errorf("static block must carry cache_control: %+v", blocks[0])
	}
	if blocks[0].CacheControl.TTL != "1h" {
		t.Errorf("static cache TTL = %q, want 1h", blocks[0].CacheControl.TTL)
	}
	if blocks[1].Text != "MEMORY" || blocks[1].CacheControl != nil {
		t.Errorf("dynamic block must NOT be cached: %+v", blocks[1])
	}
}

func TestSystemField_StaticOnlyCached(t *testing.T) {
	a := &Anthropic{extendedCache: true}
	blocks := asBlocks(t, a.systemField("PERSONA", ""))
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(blocks))
	}
	if blocks[0].CacheControl == nil {
		t.Error("lone static block should be cached")
	}
}

// When there is no static prefix, the dynamic block inherits the cache
// breakpoint to preserve the prior single-block caching behavior.
func TestSystemField_DynamicOnlyInheritsCache(t *testing.T) {
	a := &Anthropic{extendedCache: true}
	blocks := asBlocks(t, a.systemField("", "MEMORY"))
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(blocks))
	}
	if blocks[0].Text != "MEMORY" || blocks[0].CacheControl == nil {
		t.Errorf("dynamic-only block should be cached: %+v", blocks[0])
	}
}

// With caching on, exactly ONE cache breakpoint rides the tools block, on the
// LAST tool (Anthropic caches by prefix, so one breakpoint covers all preceding
// tools) and at the same 1h TTL as the system block.
func TestToAnthropicTools_CacheBreakpointOnLastTool(t *testing.T) {
	defs := []ToolDef{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	got := toAnthropicTools(defs, true)
	if len(got) != 3 {
		t.Fatalf("got %d tools, want 3", len(got))
	}
	for i := 0; i < len(got)-1; i++ {
		if got[i].CacheControl != nil {
			t.Errorf("tool %q must not carry cache_control (only the last one does)", got[i].Name)
		}
	}
	last := got[len(got)-1]
	if last.CacheControl == nil || last.CacheControl.TTL != "1h" {
		t.Errorf("last tool must carry a 1h cache breakpoint, got %+v", last.CacheControl)
	}
	// Empty schemas are still filled so the tool is valid.
	if string(got[0].InputSchema) != `{"type":"object"}` {
		t.Errorf("empty schema must default to object, got %s", got[0].InputSchema)
	}
}

// With caching off, no tool carries a breakpoint — the caching on/off policy is
// unchanged, only its granularity improves when on.
func TestToAnthropicTools_NoCacheWhenDisabled(t *testing.T) {
	got := toAnthropicTools([]ToolDef{{Name: "a"}, {Name: "b"}}, false)
	for _, tl := range got {
		if tl.CacheControl != nil {
			t.Errorf("tool %q must not be cached when extendedCache is off", tl.Name)
		}
	}
}
