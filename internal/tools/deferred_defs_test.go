package tools

import "testing"

// TestDeferredDefs: the native-tool-search catalog ships EVERY tool — eager ones
// normally, lazy/hidden ones with DeferLoading — and an activated lazy tool
// flips back to non-deferred so it stays hot.
func TestDeferredDefs(t *testing.T) {
	reg := NewRegistry(
		stubTool{name: "eager_a", desc: "always on"},
		stubTool{name: "lazy_a", desc: "on demand"},
		stubTool{name: "hidden_a", desc: "folded away"},
	)
	reg.MarkLazy("lazy_a")
	reg.MarkHidden("hidden_a")

	defs := reg.DeferredDefs(nil, nil)
	if len(defs) != 3 {
		t.Fatalf("expected the FULL catalog (3 tools), got %d", len(defs))
	}
	got := map[string]bool{}
	for _, d := range defs {
		got[d.Name] = d.DeferLoading
	}
	if got["eager_a"] {
		t.Error("eager tool must not be deferred")
	}
	if !got["lazy_a"] || !got["hidden_a"] {
		t.Errorf("lazy/hidden tools must be deferred: %+v", got)
	}

	// Activation flips a lazy tool back to non-deferred.
	defs = reg.DeferredDefs(nil, map[string]bool{"lazy_a": true})
	for _, d := range defs {
		if d.Name == "lazy_a" && d.DeferLoading {
			t.Error("activated tool must ship non-deferred")
		}
	}

	// The allow filter applies as in Defs.
	defs = reg.DeferredDefs(func(n string) bool { return n != "hidden_a" }, nil)
	for _, d := range defs {
		if d.Name == "hidden_a" {
			t.Error("allow filter ignored")
		}
	}
}
