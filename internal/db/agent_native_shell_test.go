package db

import (
	"os"
	"testing"
)

// TestAgentWithoutNativeShellFieldLoadsDisabled locks the opt-IN default of the
// native-shell toggle: an agent row that predates the field has no "nativeShell"
// key, decodes to nil and reads as disabled — the bridged shell stays the only
// shell for every existing agent.
func TestAgentWithoutNativeShellFieldLoadsDisabled(t *testing.T) {
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
	if got.NativeShell != nil {
		t.Fatalf("NativeShell = %v, want nil (key absent on disk)", *got.NativeShell)
	}
	if got.NativeShellEnabled() {
		t.Error("NativeShellEnabled() = true for an agent row without the field, want false (opt-in)")
	}
}

// TestAgentNativeShellOptInSurvivesReloadAndPatch verifies the opt-in round-trips
// through the store and that the profile patch sets it (and marks the override).
func TestAgentNativeShellOptInSurvivesReloadAndPatch(t *testing.T) {
	root := t.TempDir()
	d, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()

	created, err := d.CreateAgent(ctx, Agent{Name: "shelly", Provider: "claude-cli"})
	if err != nil {
		t.Fatal(err)
	}
	on := true
	if _, err := d.UpdateAgent(ctx, created.ID, AgentProfilePatch{NativeShell: &on}); err != nil {
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
	if !got.NativeShellEnabled() {
		t.Fatalf("NativeShellEnabled() = false after opting in, agent = %+v", got)
	}
	off := false
	if _, err := reloaded.UpdateAgent(ctx, created.ID, AgentProfilePatch{NativeShell: &off}); err != nil {
		t.Fatal(err)
	}
	got, err = reloaded.GetAgent(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.NativeShellEnabled() {
		t.Error("explicit false must switch the native shell back off")
	}
}
