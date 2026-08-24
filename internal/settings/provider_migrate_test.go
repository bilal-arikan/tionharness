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

func TestMigrateFromSettings_FullyPopulated(t *testing.T) {
	s := Settings{
		AnthropicKeyEnc: "enc:anthropic-key",

		ClaudeCLIPath:         "/usr/bin/claude",
		ClaudeConfigDir:       "/home/u/.tionharness/claude-home",
		ClaudeCliAuthKind:     "oauth",
		ClaudeCliAuthTokenEnc: "enc:claude-token",

		CodexCLIPath:   "/usr/bin/codex",
		CodexConfigDir: "/home/u/.tionharness/codex-home",

		MinimaxKeyEnc:  "enc:minimax-key",
		MinimaxBaseURL: "https://minimax.example/v1",

		OpenRouterKeyEnc:  "enc:openrouter-key",
		OpenRouterBaseURL: "https://openrouter.example/v1",

		ZAIKeyEnc:  "enc:zai-key",
		ZAIBaseURL: "https://zai.example/v1",

		DeepSeekKeyEnc:  "enc:deepseek-key",
		DeepSeekBaseURL: "https://deepseek.example/v1",

		CustomProviders: []CustomProvider{
			{ID: "my-openai", Label: "My OpenAI", Kind: "openai", BaseURL: "https://oai.example/v1", DefaultModel: "gpt-x", Models: "gpt-x,gpt-y", KeyEnc: "enc:custom-openai-key"},
			{ID: "my-claude", Label: "My Claude Compat", Kind: "anthropic", BaseURL: "https://claude-compat.example/v1", KeyEnc: "enc:custom-claude-key"},
		},
	}

	decrypt := func(enc string) string { return enc } // unused by current mappings; identity is enough
	instances := MigrateFromSettings(s, decrypt)

	wantIDs := map[string]string{ // id -> kindId
		"anthropic":          "anthropic",
		"claude-cli":         "claude-cli",
		"codex-cli":          "codex-cli",
		"minimax":            "minimax",
		"minimax-anthropic":  "minimax-anthropic",
		"openrouter":         "openrouter",
		"zai":                "zai",
		"deepseek":           "deepseek",
		"deepseek-anthropic": "deepseek-anthropic",
		"my-openai":          "openai-compat",
		"my-claude":          "anthropic-compat",
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

	anthropic, _ := findInstance(instances, "anthropic")
	if anthropic.SecretsEnc["key"] != "enc:anthropic-key" {
		t.Fatalf("anthropic secret not migrated: %+v", anthropic.SecretsEnc)
	}

	cli, _ := findInstance(instances, "claude-cli")
	if cli.Config["cliPath"] != "/usr/bin/claude" || cli.Config["configDir"] != "/home/u/.tionharness/claude-home" || cli.Config["authKind"] != "oauth" {
		t.Fatalf("claude-cli config not migrated: %+v", cli.Config)
	}
	if cli.SecretsEnc["authToken"] != "enc:claude-token" {
		t.Fatalf("claude-cli authToken not migrated: %+v", cli.SecretsEnc)
	}

	codex, _ := findInstance(instances, "codex-cli")
	if codex.Config["cliPath"] != "/usr/bin/codex" || codex.Config["configDir"] != "/home/u/.tionharness/codex-home" {
		t.Fatalf("codex-cli config not migrated: %+v", codex.Config)
	}

	minimax, _ := findInstance(instances, "minimax")
	if minimax.Config["baseUrl"] != "https://minimax.example/v1" || minimax.SecretsEnc["key"] != "enc:minimax-key" {
		t.Fatalf("minimax not migrated correctly: %+v", minimax)
	}

	// The Anthropic-mode variants share the base provider's key but NOT its base
	// URL: the migrated baseUrl is the OpenAI-compatible endpoint, which the
	// Anthropic transport cannot talk to. Empty means "use the kind's own default".
	minimaxAnthropic, _ := findInstance(instances, "minimax-anthropic")
	if minimaxAnthropic.SecretsEnc["key"] != "enc:minimax-key" || minimaxAnthropic.Config["baseUrl"] != "" {
		t.Fatalf("minimax-anthropic not migrated correctly: %+v", minimaxAnthropic)
	}

	deepseekAnthropic, _ := findInstance(instances, "deepseek-anthropic")
	if deepseekAnthropic.SecretsEnc["key"] != "enc:deepseek-key" || deepseekAnthropic.Config["baseUrl"] != "" {
		t.Fatalf("deepseek-anthropic not migrated correctly: %+v", deepseekAnthropic)
	}

	customOpenAI, _ := findInstance(instances, "my-openai")
	if customOpenAI.Config["baseUrl"] != "https://oai.example/v1" || customOpenAI.DefaultModel != "gpt-x" || customOpenAI.Models != "gpt-x,gpt-y" {
		t.Fatalf("custom openai-compat not migrated correctly: %+v", customOpenAI)
	}
	if customOpenAI.SecretsEnc["key"] != "enc:custom-openai-key" {
		t.Fatalf("custom openai-compat secret not migrated: %+v", customOpenAI.SecretsEnc)
	}

	customClaude, _ := findInstance(instances, "my-claude")
	if customClaude.KindID != "anthropic-compat" {
		t.Fatalf("expected anthropic-compat kind, got %q", customClaude.KindID)
	}
}

func TestMigrateFromSettings_CustomProviderReplacesBuiltinWithSameID(t *testing.T) {
	s := Settings{
		OpenRouterKeyEnc:  "enc:openrouter-key",
		OpenRouterBaseURL: "https://openrouter.example/v1",
		CustomProviders: []CustomProvider{
			{ID: "openrouter", Label: "My OpenRouter", Kind: "openai", BaseURL: "https://openrouter.ai/api/v1", KeyEnc: "enc:custom-openrouter-key"},
		},
	}

	instances := MigrateFromSettings(s, func(enc string) string { return enc })

	count := 0
	for _, inst := range instances {
		if inst.ID == "openrouter" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 instance with id %q, got %d: %+v", "openrouter", count, instances)
	}

	inst, ok := findInstance(instances, "openrouter")
	if !ok {
		t.Fatal("missing openrouter instance")
	}
	if inst.KindID != "openai-compat" {
		t.Fatalf("expected custom provider to win with kindId openai-compat, got %q", inst.KindID)
	}
	if inst.Config["baseUrl"] != "https://openrouter.ai/api/v1" {
		t.Fatalf("expected custom provider's baseUrl to win, got %+v", inst.Config)
	}
	if inst.SecretsEnc["key"] != "enc:custom-openrouter-key" {
		t.Fatalf("expected custom provider's secret to win, got %+v", inst.SecretsEnc)
	}
}

func TestMigrateFromSettings_EmptySettings(t *testing.T) {
	instances := MigrateFromSettings(Settings{}, func(enc string) string { return enc })

	// claude-cli always migrates (keyless-capable); nothing else should.
	if len(instances) != 1 {
		t.Fatalf("expected exactly 1 instance (claude-cli) for empty settings, got %d: %+v", len(instances), instances)
	}
	cli, ok := findInstance(instances, "claude-cli")
	if !ok {
		t.Fatal("expected claude-cli instance even with empty settings")
	}
	if cli.KindID != "claude-cli" {
		t.Fatalf("expected kindId claude-cli, got %q", cli.KindID)
	}
}

func TestMigrateFromSettings_CodexOnlyWhenConfigured(t *testing.T) {
	instances := MigrateFromSettings(Settings{CodexCLIPath: "/usr/bin/codex"}, func(enc string) string { return enc })
	if _, ok := findInstance(instances, "codex-cli"); !ok {
		t.Fatal("expected codex-cli instance when CodexCLIPath is set")
	}

	instances = MigrateFromSettings(Settings{}, func(enc string) string { return enc })
	if _, ok := findInstance(instances, "codex-cli"); ok {
		t.Fatal("expected no codex-cli instance when neither codex field is set")
	}
}

func TestProviderStore_EnsureMigrated_Idempotent(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenProviderStore(dir, testCipher{})
	if err != nil {
		t.Fatalf("OpenProviderStore: %v", err)
	}

	sett := Settings{AnthropicKeyEnc: "enc:anthropic-key"}
	decrypt := func(enc string) string { return enc }

	migrated, err := s.EnsureMigrated(sett, decrypt)
	if err != nil {
		t.Fatalf("EnsureMigrated: %v", err)
	}
	if !migrated {
		t.Fatal("expected first EnsureMigrated call to report migrated=true")
	}
	if _, ok := s.Get("anthropic"); !ok {
		t.Fatal("expected anthropic instance after migration")
	}

	// Simulate a user editing/deleting an instance after migration, then
	// re-running EnsureMigrated (e.g. a process restart) — it must NOT
	// resurrect or overwrite what's now on disk.
	if err := s.Delete("anthropic"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	migratedAgain, err := s.EnsureMigrated(sett, decrypt)
	if err != nil {
		t.Fatalf("second EnsureMigrated: %v", err)
	}
	if migratedAgain {
		t.Fatal("expected second EnsureMigrated call to report migrated=false")
	}
	if _, ok := s.Get("anthropic"); ok {
		t.Fatal("expected EnsureMigrated to NOT resurrect a deleted instance on a second call")
	}

	// And a freshly reopened store (new process) must also see the file as
	// already-migrated and refuse to touch it.
	reopened, err := OpenProviderStore(dir, testCipher{})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	migratedOnReopen, err := reopened.EnsureMigrated(sett, decrypt)
	if err != nil {
		t.Fatalf("EnsureMigrated on reopened store: %v", err)
	}
	if migratedOnReopen {
		t.Fatal("expected EnsureMigrated on a reopened, already-migrated store to report false")
	}
}
