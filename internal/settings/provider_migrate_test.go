package settings

import "testing"

func findInstance(list []ProviderInstance, id string) (ProviderInstance, bool) {
	for _, p := range list {
		if p.ID == id {
			return p, true
		}
	}
	return ProviderInstance{}, false
}

// TestEnsureSeeded_WritesDefaultCLIInstancesOnce: a fresh install starts with the
// keyless claude-cli and codex-cli instances on their app-global homes; a store
// whose providers.json already existed is never reseeded.
func TestEnsureSeeded_WritesDefaultCLIInstancesOnce(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenProviderStore(dir, testCipher{})
	if err != nil {
		t.Fatal(err)
	}
	seeded, err := store.EnsureSeeded()
	if err != nil || !seeded {
		t.Fatalf("first seed: seeded=%v err=%v", seeded, err)
	}
	instances := store.List()
	if len(instances) != 2 {
		t.Fatalf("expected claude-cli + codex-cli, got %+v", instances)
	}
	for _, id := range []string{"claude-cli", "codex-cli"} {
		inst, ok := findInstance(instances, id)
		if !ok || inst.KindID != id || !inst.Enabled {
			t.Fatalf("instance %q = %+v ok=%v", id, inst, ok)
		}
	}
	if codex, _ := findInstance(instances, "codex-cli"); codex.Config["configDir"] != "" {
		t.Fatalf("codex-cli must use the app-global home, got configDir %q", codex.Config["configDir"])
	}
	if seeded, err := store.EnsureSeeded(); err != nil || seeded {
		t.Fatalf("second seed on the same store: seeded=%v err=%v", seeded, err)
	}

	reopened, err := OpenProviderStore(dir, testCipher{})
	if err != nil {
		t.Fatal(err)
	}
	if seeded, err := reopened.EnsureSeeded(); err != nil || seeded {
		t.Fatalf("seed over an existing providers.json: seeded=%v err=%v", seeded, err)
	}
}

// TestEnsureKindInstance_AddsOnceForMissingKind: a store that predates a kind
// gains exactly one default instance of it; later calls are no-ops.
func TestEnsureKindInstance_AddsOnceForMissingKind(t *testing.T) {
	store, err := OpenProviderStore(t.TempDir(), testCipher{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnsureSeeded(); err != nil {
		t.Fatal(err)
	}
	added, err := store.EnsureKindInstance("opencode-cli", "OpenCode CLI")
	if err != nil || !added {
		t.Fatalf("first call: added=%v err=%v", added, err)
	}
	inst, ok := store.Get("opencode-cli")
	if !ok || inst.KindID != "opencode-cli" || !inst.Enabled || inst.Label != "OpenCode CLI" {
		t.Fatalf("instance = %+v ok=%v", inst, ok)
	}
	if added, err := store.EnsureKindInstance("opencode-cli", "OpenCode CLI"); err != nil || added {
		t.Fatalf("second call: added=%v err=%v", added, err)
	}
	if n := len(store.List()); n != 3 {
		t.Fatalf("instances = %d, want claude-cli + codex-cli + opencode-cli", n)
	}
}
