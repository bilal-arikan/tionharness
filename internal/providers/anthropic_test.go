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
