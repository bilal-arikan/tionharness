package db

import (
	"os"
	"testing"
)

// TestAgentWithoutNativeWebSearchFieldLoadsEnabled locks the DEFAULT of the
// provider-native web-search toggle for rows that predate it. Every agent file
// written before the field existed simply has no "nativeWebSearch" key, so the
// only way "default on" can be true is for the absent key to decode to nil and
// for NativeWebSearchEnabled to read nil as enabled. A plain bool field would
// decode to false here and silently disable native search for every existing
// agent — which is exactly the regression this test exists to catch.
func TestAgentWithoutNativeWebSearchFieldLoadsEnabled(t *testing.T) {
	root := t.TempDir()
	d, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()

	created, err := d.CreateAgent(ctx, Agent{Name: "legacy", Provider: "claude-cli"})
	if err != nil {
		t.Fatal(err)
	}
	path, err := d.AgentPath(created.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Replace the row with the pre-toggle on-disk shape: no nativeWebSearch key
	// at all. Written by hand rather than re-marshalled, so the test keeps
	// asserting against the literal legacy JSON even if the struct grows.
	legacy := `{"id":"` + created.ID + `","name":"legacy","provider":"claude-cli",` +
		`"providerInstanceId":"claude-cli","permissionMode":"auto","mcpEnabled":true}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}

	reloaded, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reloaded.GetAgent(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.NativeWebSearch != nil {
		t.Fatalf("NativeWebSearch = %v, want nil (key absent on disk)", *got.NativeWebSearch)
	}
	if !got.NativeWebSearchEnabled() {
		t.Error("NativeWebSearchEnabled() = false for an agent row without the field, want true (nil means enabled)")
	}
}

// TestAgentNativeWebSearchExplicitFalseSurvivesReload verifies the opt-out is
// actually persisted: an explicit false must round-trip through the store, not
// be swallowed by `omitempty` the way a plain bool false would be.
func TestAgentNativeWebSearchExplicitFalseSurvivesReload(t *testing.T) {
	root := t.TempDir()
	d, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()

	created, err := d.CreateAgent(ctx, Agent{Name: "opted-out", Provider: "claude-cli"})
	if err != nil {
		t.Fatal(err)
	}
	off := false
	if _, err := d.UpdateAgent(ctx, created.ID, AgentProfilePatch{NativeWebSearch: &off}); err != nil {
		t.Fatal(err)
	}

	reloaded, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reloaded.GetAgent(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.NativeWebSearch == nil || *got.NativeWebSearch {
		t.Fatalf("NativeWebSearch = %v, want a stored false", got.NativeWebSearch)
	}
	if got.NativeWebSearchEnabled() {
		t.Error("NativeWebSearchEnabled() = true after an explicit opt-out")
	}
}
