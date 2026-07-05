package tools

import "testing"

// TestBridgeableDefsFilteredSkipsHidden verifies the POC hidden-tier exclusion:
// with skipHidden=false the hidden self-management tool is bridged (default,
// == BridgeableDefs), and with skipHidden=true it is withheld while summary/
// name-only lazy tools still bridge. HiddenBridgeableCount reports the delta.
func TestBridgeableDefsFilteredSkipsHidden(t *testing.T) {
	reg := NewRegistry(
		stubTool{name: "eager_a", desc: "always on"},
		stubTool{name: "summary_tool", desc: "lazy summary tier"},
		stubTool{name: "nameonly_tool", desc: "lazy name-only tier"},
		stubTool{name: "hidden_tool", desc: "hidden self-mgmt tier"},
	)
	reg.MarkLazy("summary_tool")     // summary tier (lazy, not hidden/nameOnly)
	reg.MarkNameOnly("nameonly_tool") // name-only tier
	reg.MarkHidden("hidden_tool")     // hidden tier

	// Default (skipHidden=false) must equal BridgeableDefs and include the hidden tool.
	base := names(reg.BridgeableDefsFiltered(nil, false))
	if !eq(base, []string{"hidden_tool", "nameonly_tool", "summary_tool"}) {
		t.Fatalf("skipHidden=false = %v, want all three lazy tools", base)
	}
	if !eq(base, names(reg.BridgeableDefs(nil))) {
		t.Errorf("BridgeableDefsFiltered(_, false) must equal BridgeableDefs")
	}

	// skipHidden=true drops only the hidden tier; summary + name-only still bridge.
	kept := names(reg.BridgeableDefsFiltered(nil, true))
	if !eq(kept, []string{"nameonly_tool", "summary_tool"}) {
		t.Errorf("skipHidden=true = %v, want [nameonly_tool summary_tool]", kept)
	}

	// The measurement helper reports exactly what was dropped.
	if n := reg.HiddenBridgeableCount(nil); n != 1 {
		t.Errorf("HiddenBridgeableCount = %d, want 1", n)
	}

	// Withheld tools still ship full schemas when NOT skipped (regression guard).
	for _, d := range reg.BridgeableDefsFiltered(nil, false) {
		if len(d.InputSchema) == 0 {
			t.Errorf("bridged tool %s must carry a full schema", d.Name)
		}
	}
}

// TestBridgeableDefsIncludesFullSelfManaged verifies a self-management tool promoted
// to the full tier (lazy flag cleared) is STILL bridged, via the stable selfManaged
// stamp. Without it, a full-tier self-management tool would vanish from the CLI
// (BridgeableDefs bridges lazy built-ins only) — the create_agent-full regression.
func TestBridgeableDefsIncludesFullSelfManaged(t *testing.T) {
	reg := NewRegistry(
		stubTool{name: "create_agent", desc: "self-mgmt"},
		stubTool{name: "todo_write", desc: "eager behavioral"},
	)
	reg.MarkSelfManaged("create_agent") // stable membership
	reg.MarkHidden("create_agent")      // default hidden tier
	reg.SetVisibility("create_agent", VisibilityFull) // user promotes to full (clears lazy)

	got := names(reg.BridgeableDefsFiltered(nil, true))
	if !eq(got, []string{"create_agent"}) {
		t.Fatalf("full-tier self-managed tool must still bridge, got %v", got)
	}
	// A plain eager built-in that is NOT self-managed must NOT be bridged (it rides
	// the static specs or the CLI's own equivalent).
	if contains2(got, "todo_write") {
		t.Errorf("non-self-managed eager tool must not leak into the bridge: %v", got)
	}
}

func contains2(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
