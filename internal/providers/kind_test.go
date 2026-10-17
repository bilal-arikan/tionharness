package providers

import (
	"slices"
	"testing"
)

// instanceOf builds a minimal registry Instance for a built-in kind id (kind
// id == instance id, mirroring the default migrated instances, _Docs/71 §3).
func instanceOf(kindID string, values map[string]string) Instance {
	return Instance{ID: kindID, KindID: kindID, Values: values}
}

// TestCatalogDerivedFromKinds verifies the catalog is built from the registered
// kinds in Order and carries the expected built-in trio.
func TestCatalogDerivedFromKinds(t *testing.T) {
	cat := Catalog()
	if len(cat) != 3 {
		t.Fatalf("catalog: want 3 entries, got %d", len(cat))
	}
	wantOrder := []string{"claude-cli", "codex-cli", "opencode-cli"}
	for i, id := range wantOrder {
		if cat[i].ID != id {
			t.Errorf("catalog[%d].ID = %q, want %q", i, cat[i].ID, id)
		}
	}
	if len(cat[0].Models) == 0 {
		t.Error("claude-cli entry has no models")
	}
}

func TestCatalogCodexGPT5HasFullThinkingRamp(t *testing.T) {
	for _, entry := range Catalog() {
		if entry.ID != "codex-cli" {
			continue
		}
		for _, model := range entry.Models {
			if model.ID == "gpt-5.6-sol" {
				want := []string{"off", "low", "medium", "high", "xhigh", "max", "ultra"}
				if !slices.Equal(model.ThinkingTiers, want) || model.ThinkingClass != "adaptive" {
					t.Fatalf("gpt-5.6-sol metadata = tiers %v class %q", model.ThinkingTiers, model.ThinkingClass)
				}
				return
			}
		}
	}
	t.Fatal("codex-cli/gpt-5.6-sol missing from catalog")
}

// TestRegistryGetDispatchesByKind checks Get builds the right concrete provider
// per kind and that availability reflects configuration.
func TestRegistryGetDispatchesByKind(t *testing.T) {
	r := NewRegistry()
	r.claudeCLIPath = "/usr/bin/claude" // simulate CLI present
	r.SetInstances([]Instance{
		instanceOf("codex-cli", map[string]string{FieldKeyCLIPath: "/usr/bin/codex"}),
		instanceOf("opencode-cli", map[string]string{FieldKeyCLIPath: "/usr/bin/opencode"}),
		instanceOf("claude-cli", nil),
	})

	cases := []struct {
		name     string
		wantName string
	}{
		{"codex-cli", "codex-cli"},
		{"opencode-cli", "opencode-cli"},
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
	r.codexCLIPath = ""
	if r.Available("codex-cli") {
		t.Error("codex-cli available without a registered instance")
	}
	if r.Available("opencode-cli") {
		t.Error("opencode-cli available without a registered instance")
	}
	r.SetInstances([]Instance{instanceOf("codex-cli", nil)})
	if r.Available("codex-cli") {
		t.Error("codex-cli available without a binary")
	}
	r.SetInstances([]Instance{instanceOf("codex-cli", map[string]string{FieldKeyCLIPath: "/usr/bin/codex"})})
	if !r.Available("codex-cli") {
		t.Error("codex-cli not available after its binary path is set")
	}
	if r.Available("nope") {
		t.Error("unknown id reported available")
	}
}

// TestTwoInstancesOfSameKindAreIndependent verifies the Faz 2 payoff: two
// provider instances of the SAME kind (two "claude-cli" instances, different
// credentials) resolve to independent clients carrying their own credential,
// neither leaking into the other (_Docs/71 §10 test plan).
func TestTwoInstancesOfSameKindAreIndependent(t *testing.T) {
	r := NewRegistry()
	r.SetInstances([]Instance{
		{ID: "claude-cli", KindID: "claude-cli", Values: map[string]string{FieldKeyCLIPath: "/usr/bin/claude", FieldKeyAuthKind: "oauth", FieldKeyAuthToken: "tok-work"}},
		{ID: "PRV1", KindID: "claude-cli", Values: map[string]string{FieldKeyCLIPath: "/usr/bin/claude", FieldKeyAuthKind: "oauth", FieldKeyAuthToken: "tok-personal"}},
	})

	work, err := r.Get("claude-cli")
	if err != nil {
		t.Fatalf("Get(claude-cli): %v", err)
	}
	personal, err := r.Get("PRV1")
	if err != nil {
		t.Fatalf("Get(PRV1): %v", err)
	}
	workClient, ok := work.(*ClaudeCLI)
	if !ok {
		t.Fatalf("Get(claude-cli) did not return *ClaudeCLI")
	}
	personalClient, ok := personal.(*ClaudeCLI)
	if !ok {
		t.Fatalf("Get(PRV1) did not return *ClaudeCLI")
	}
	if workClient.authToken != "tok-work" {
		t.Errorf("claude-cli instance authToken = %q, want tok-work", workClient.authToken)
	}
	if personalClient.authToken != "tok-personal" {
		t.Errorf("PRV1 instance authToken = %q, want tok-personal", personalClient.authToken)
	}

	if !r.Available("claude-cli") || !r.Available("PRV1") {
		t.Error("both instances should be available with their own binary path set")
	}
	if got := r.KindOf("PRV1"); got != "claude-cli" {
		t.Errorf("KindOf(PRV1) = %q, want claude-cli", got)
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
		{ID: "claude-cli", KindID: "claude-cli", Enabled: true, Label: "Claude — iş", Values: map[string]string{FieldKeyCLIPath: "/usr/bin/claude"}},
		{ID: "PRV1", KindID: "claude-cli", Enabled: true, Label: "Claude — kişisel", Values: map[string]string{FieldKeyCLIPath: "/usr/bin/claude"}},
		{ID: "PRV2", KindID: "claude-cli", Enabled: false, Label: "Claude — devre dışı"},
		{ID: "ghost", KindID: "does-not-exist-kind", Enabled: true},
	})

	cat := r.InstanceCatalog()
	byID := make(map[string]CatalogEntry, len(cat))
	for _, e := range cat {
		byID[e.ID] = e
	}

	if _, ok := byID["claude-cli"]; !ok {
		t.Error("missing catalog entry for default claude-cli instance")
	}
	if _, ok := byID["PRV1"]; !ok {
		t.Error("missing catalog entry for second claude-cli instance (PRV1)")
	}
	if byID["claude-cli"].Label == byID["PRV1"].Label {
		t.Error("two distinct instances of the same kind must not collapse into one catalog entry")
	}
	if _, ok := byID["PRV2"]; ok {
		t.Error("disabled instance PRV2 must not appear in the catalog")
	}
	if _, ok := byID["ghost"]; ok {
		t.Error("instance with an unregistered kind must not appear in the catalog")
	}
	if byID["PRV1"].NeedsKey {
		t.Error("PRV1 entry should inherit NeedsKey=false from the claude-cli kind manifest")
	}
	if len(byID["PRV1"].Models) == 0 {
		t.Error("PRV1 entry should inherit the claude-cli kind's curated model list (no instance override set)")
	}
}

// TestInstanceCatalogEnrichesModels guards the regression where instance-derived
// entries shipped raw manifest models — MergeCatalog lets them override the
// enriched per-kind entry, so the UI lost every contextWindow/maxOutput.
func TestInstanceCatalogEnrichesModels(t *testing.T) {
	r := NewRegistry()
	r.SetInstances([]Instance{
		{ID: "claude-cli", KindID: "claude-cli", Enabled: true, Values: map[string]string{FieldKeyCLIPath: "/usr/bin/claude"}},
		{ID: "PRV1", KindID: "claude-cli", Enabled: true, Models: "claude-opus-5", Values: map[string]string{FieldKeyCLIPath: "/usr/bin/claude"}},
	})

	byID := make(map[string]CatalogEntry)
	for _, e := range r.InstanceCatalog() {
		byID[e.ID] = e
	}

	for _, m := range byID["claude-cli"].Models {
		if m.ID == "" {
			continue // the "session model" alias has no family of its own
		}
		if m.ContextWindow == 0 {
			t.Errorf("manifest model %q: ContextWindow = 0, want family value", m.ID)
		}
		if m.MaxOutput == 0 {
			t.Errorf("manifest model %q: MaxOutput = 0, want family value", m.ID)
		}
	}

	custom := byID["PRV1"].Models
	if len(custom) != 1 || custom[0].ID != "claude-opus-5" {
		t.Fatalf("PRV1 models = %+v, want the custom list", custom)
	}
	if custom[0].ContextWindow == 0 || custom[0].MaxOutput == 0 {
		t.Errorf("custom model not enriched: %+v", custom[0])
	}
	if custom[0].ThinkingClass == "" || len(custom[0].ThinkingTiers) == 0 {
		t.Errorf("custom model missing thinking metadata: %+v", custom[0])
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
	if got := byID["claude-cli"].AppliesToolHooks; !got {
		t.Error("openrouter.AppliesToolHooks = false, want true (native tool loop runs hooks)")
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
