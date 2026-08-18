package providers

import "testing"

// instanceOf builds a minimal registry Instance for a built-in kind id (kind
// id == instance id, mirroring the default migrated instances, _Docs/71 §3).
func instanceOf(kindID string, values map[string]string) Instance {
	return Instance{ID: kindID, KindID: kindID, Values: values}
}

// TestCatalogDerivedFromKinds verifies the catalog is built from the registered
// kinds in Order and carries the expected built-in trio.
func TestCatalogDerivedFromKinds(t *testing.T) {
	cat := Catalog()
	if len(cat) != 11 {
		t.Fatalf("catalog: want 11 entries, got %d", len(cat))
	}
	wantOrder := []string{"claude-cli", "anthropic", "minimax", "minimax-anthropic", "openrouter", "zai", "deepseek", "deepseek-anthropic", "codex-cli", "openai-compat", "anthropic-compat"}
	for i, id := range wantOrder {
		if cat[i].ID != id {
			t.Errorf("catalog[%d].ID = %q, want %q", i, cat[i].ID, id)
		}
	}
	if len(cat[0].Models) == 0 {
		t.Error("claude-cli entry has no models")
	}
}

// TestRegistryGetDispatchesByKind checks Get builds the right concrete provider
// per kind and that availability reflects configuration.
func TestRegistryGetDispatchesByKind(t *testing.T) {
	r := NewRegistry()
	r.claudeCLIPath = "/usr/bin/claude" // simulate CLI present
	r.SetInstances([]Instance{
		instanceOf("anthropic", map[string]string{FieldKeyAPIKey: "sk-test"}),
		instanceOf("minimax", map[string]string{FieldKeyAPIKey: "mm-test"}),
		instanceOf("minimax-anthropic", map[string]string{FieldKeyAPIKey: "mm-test"}),
		instanceOf("claude-cli", nil),
	})

	cases := []struct {
		name     string
		wantName string
	}{
		{"anthropic", "anthropic"},
		{"minimax", "minimax"},
		{"minimax-anthropic", "minimax-anthropic"}, // reuses MiniMax key, Anthropic transport
		{"claude-cli", "claude-cli"},
		{"", "claude-cli"}, // empty normalizes to the CLI default
	}
	for _, c := range cases {
		p, err := r.Get(c.name)
		if err != nil {
			t.Fatalf("Get(%q): %v", c.name, err)
		}
		if p.Name() != c.wantName {
			t.Errorf("Get(%q).Name() = %q, want %q", c.name, p.Name(), c.wantName)
		}
	}

	if _, err := r.Get("does-not-exist"); err == nil {
		t.Error("Get(unknown): want error, got nil")
	}
}

// TestRegistryAvailable verifies per-id availability gating.
func TestRegistryAvailable(t *testing.T) {
	r := NewRegistry() // no instances registered
	if r.Available("anthropic") {
		t.Error("anthropic available without a registered instance")
	}
	if r.Available("minimax") {
		t.Error("minimax available without a registered instance")
	}
	r.SetInstances([]Instance{instanceOf("anthropic", map[string]string{FieldKeyAPIKey: "sk-x"})})
	if !r.Available("anthropic") {
		t.Error("anthropic not available after key set")
	}
	if r.Available("nope") {
		t.Error("unknown id reported available")
	}
}

// TestTwoInstancesOfSameKindAreIndependent verifies the Faz 2 payoff: two
// provider instances of the SAME kind (two "anthropic" instances, different
// keys) resolve to independent clients carrying their own credential, neither
// leaking into the other (_Docs/71 §10 test plan).
func TestTwoInstancesOfSameKindAreIndependent(t *testing.T) {
	r := NewRegistry()
	r.SetInstances([]Instance{
		{ID: "anthropic", KindID: "anthropic", Values: map[string]string{FieldKeyAPIKey: "sk-work"}},
		{ID: "PRV1", KindID: "anthropic", Values: map[string]string{FieldKeyAPIKey: "sk-personal"}},
	})

	work, err := r.Get("anthropic")
	if err != nil {
		t.Fatalf("Get(anthropic): %v", err)
	}
	personal, err := r.Get("PRV1")
	if err != nil {
		t.Fatalf("Get(PRV1): %v", err)
	}
	workAnthropic, ok := work.(*Anthropic)
	if !ok {
		t.Fatalf("Get(anthropic) did not return *Anthropic")
	}
	personalAnthropic, ok := personal.(*Anthropic)
	if !ok {
		t.Fatalf("Get(PRV1) did not return *Anthropic")
	}
	if workAnthropic.apiKey != "sk-work" {
		t.Errorf("anthropic instance apiKey = %q, want sk-work", workAnthropic.apiKey)
	}
	if personalAnthropic.apiKey != "sk-personal" {
		t.Errorf("PRV1 instance apiKey = %q, want sk-personal", personalAnthropic.apiKey)
	}

	if !r.Available("anthropic") || !r.Available("PRV1") {
		t.Error("both instances should be available with their own key set")
	}
	if got := r.KindOf("PRV1"); got != "anthropic" {
		t.Errorf("KindOf(PRV1) = %q, want anthropic", got)
	}
}

// TestGetUnregisteredKindErrors verifies an instance whose KindID no longer
// matches a registered kind (a kind was removed, or the instance is corrupt)
// errors instead of panicking or falling back silently (_Docs/71 §4.3).
func TestGetUnregisteredKindErrors(t *testing.T) {
	r := NewRegistry()
	r.SetInstances([]Instance{{ID: "ghost", KindID: "does-not-exist-kind"}})
	if _, err := r.Get("ghost"); err == nil {
		t.Error("Get(ghost): want error for unregistered kind, got nil")
	}
	if r.Available("ghost") {
		t.Error("Available(ghost): want false for unregistered kind")
	}
}

// TestInstanceCatalogPerInstanceEntries verifies InstanceCatalog gives two
// same-kind instances their own catalog entry (Faz 5, _Docs/71 §5 item 3),
// skips a disabled instance and an instance whose kind is unregistered, and
// carries the kind's manifest metadata (needsKey/models) onto the entry.
func TestInstanceCatalogPerInstanceEntries(t *testing.T) {
	r := NewRegistry()
	r.SetInstances([]Instance{
		{ID: "anthropic", KindID: "anthropic", Enabled: true, Label: "Anthropic — iş", Values: map[string]string{FieldKeyAPIKey: "sk-work"}},
		{ID: "PRV1", KindID: "anthropic", Enabled: true, Label: "Anthropic — kişisel", Values: map[string]string{FieldKeyAPIKey: "sk-personal"}},
		{ID: "PRV2", KindID: "anthropic", Enabled: false, Label: "Anthropic — devre dışı"},
		{ID: "ghost", KindID: "does-not-exist-kind", Enabled: true},
	})

	cat := r.InstanceCatalog()
	byID := make(map[string]CatalogEntry, len(cat))
	for _, e := range cat {
		byID[e.ID] = e
	}

	if _, ok := byID["anthropic"]; !ok {
		t.Error("missing catalog entry for default anthropic instance")
	}
	if _, ok := byID["PRV1"]; !ok {
		t.Error("missing catalog entry for second anthropic instance (PRV1)")
	}
	if byID["anthropic"].Label == byID["PRV1"].Label {
		t.Error("two distinct instances of the same kind must not collapse into one catalog entry")
	}
	if _, ok := byID["PRV2"]; ok {
		t.Error("disabled instance PRV2 must not appear in the catalog")
	}
	if _, ok := byID["ghost"]; ok {
		t.Error("instance with an unregistered kind must not appear in the catalog")
	}
	if !byID["PRV1"].NeedsKey {
		t.Error("PRV1 entry should inherit NeedsKey=true from the anthropic kind manifest")
	}
	if len(byID["PRV1"].Models) == 0 {
		t.Error("PRV1 entry should inherit the anthropic kind's curated model list (no instance override set)")
	}
}

// TestAppliesToolHooksSignal verifies the provider-capability signal used to
// surface the codex-cli hook gap in the UI: codex-cli reports hooks as NOT
// applied (its subprocess tool loop has no hook passthrough), while claude-cli
// and a native HTTP kind report hooks as applied.
func TestAppliesToolHooksSignal(t *testing.T) {
	cat := Catalog()
	byID := make(map[string]CatalogEntry, len(cat))
	for _, e := range cat {
		byID[e.ID] = e
	}

	if got := byID["codex-cli"].AppliesToolHooks; got {
		t.Error("codex-cli.AppliesToolHooks = true, want false (no hook passthrough)")
	}
	if got := byID["claude-cli"].AppliesToolHooks; !got {
		t.Error("claude-cli.AppliesToolHooks = false, want true (hooks forwarded via --settings)")
	}
	if got := byID["anthropic"].AppliesToolHooks; !got {
		t.Error("anthropic.AppliesToolHooks = false, want true (native tool loop runs hooks)")
	}
}

// TestGetUnconfiguredReturnsError preserves the historical "not configured"
// errors from each kind's Build.
func TestGetUnconfiguredReturnsError(t *testing.T) {
	r := NewRegistry() // no keys, force-clear CLI path
	r.claudeCLIPath = ""
	r.SetInstances([]Instance{
		instanceOf("anthropic", nil),
		instanceOf("minimax", nil),
		instanceOf("claude-cli", nil),
	})
	for _, id := range []string{"anthropic", "minimax", "claude-cli"} {
		if _, err := r.Get(id); err == nil {
			t.Errorf("Get(%q): want unconfigured error, got nil", id)
		}
	}
}
