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

// TestMigrateFromSettings_SeedsBuiltInInstances: the boot seed derives the
// claude-cli / codex-cli instances from the app settings. claude-cli is always
// present (it runs keyless), codex-cli only when configured.
func TestMigrateFromSettings_SeedsBuiltInInstances(t *testing.T) {
	s := Settings{
		ClaudeCLIPath:         "/usr/bin/claude",
		ClaudeCliAuthKind:     "oauth",
		ClaudeCliAuthTokenEnc: "enc:claude-token",

		CodexCLIPath:   "/usr/bin/codex",
		CodexConfigDir: "/home/u/.tionharness/codex-home",
	}

	instances := MigrateFromSettings(s, func(enc string) string { return enc })

	wantIDs := map[string]string{ // id -> kindId
		"claude-cli": "claude-cli",
		"codex-cli":  "codex-cli",
	}
	if len(instances) != len(wantIDs) {
		t.Fatalf("expected %d instances, got %d: %+v", len(wantIDs), len(instances), instances)
	}
	for id, kind := range wantIDs {
		inst, ok := findInstance(instances, id)
		if !ok {
			t.Fatalf("missing expected instance %q", id)
		}
		if inst.KindID != kind {
			t.Fatalf("instance %q: expected kindId %q, got %q", id, kind, inst.KindID)
		}
	}

	cli, _ := findInstance(instances, "claude-cli")
	if cli.Config["cliPath"] != "/usr/bin/claude" || cli.Config["configDir"] != defaultClaudeConfigDir() || cli.Config["authKind"] != "oauth" {
		t.Fatalf("claude-cli config not migrated: %+v", cli.Config)
	}
	if cli.SecretsEnc["authToken"] != "enc:claude-token" {
		t.Fatalf("claude-cli authToken not migrated: %+v", cli.SecretsEnc)
	}

	codex, _ := findInstance(instances, "codex-cli")
	if codex.Config["cliPath"] != "/usr/bin/codex" || codex.Config["configDir"] != "/home/u/.tionharness/codex-home" {
		t.Fatalf("codex-cli config not migrated: %+v", codex.Config)
	}
}

// TestMigrateFromSettings_EmptySettingsStillSeedClaudeCLI: a fresh install has
// nothing configured, yet the keyless claude-cli instance must exist so the
// first agent has a provider to run on.
func TestMigrateFromSettings_EmptySettingsStillSeedClaudeCLI(t *testing.T) {
	instances := MigrateFromSettings(Settings{}, func(enc string) string { return enc })
	if len(instances) != 1 {
		t.Fatalf("expected only claude-cli, got %+v", instances)
	}
	if instances[0].ID != "claude-cli" || !instances[0].Enabled {
		t.Fatalf("unexpected seed instance: %+v", instances[0])
	}
}

// TestEnsureKindInstance_AddsOnceForMissingKind: a store that predates a kind
// gains exactly one default instance of it; later calls are no-ops.
func TestEnsureKindInstance_AddsOnceForMissingKind(t *testing.T) {
	store, err := OpenProviderStore(t.TempDir(), testCipher{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnsureMigrated(Settings{}, func(enc string) string { return enc }); err != nil {
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
	if n := len(store.List()); n != 2 {
		t.Fatalf("instances = %d, want claude-cli + opencode-cli", n)
	}
}
