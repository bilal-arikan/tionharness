package providers

import "testing"

// TestCatalogDerivedFromKinds verifies the catalog is built from the registered
// kinds in Order and carries the expected built-in trio.
func TestCatalogDerivedFromKinds(t *testing.T) {
	cat := Catalog()
	if len(cat) != 6 {
		t.Fatalf("catalog: want 6 entries, got %d", len(cat))
	}
	wantOrder := []string{"claude-cli", "anthropic", "minimax", "minimax-anthropic", "openrouter", "zai"}
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
	r := NewRegistry("sk-test")
	r.SetMinimax("mm-test", "")
	r.claudeCLIPath = "/usr/bin/claude" // simulate CLI present

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
	r := NewRegistry("") // no anthropic key
	if r.Available("anthropic") {
		t.Error("anthropic available without key")
	}
	if r.Available("minimax") {
		t.Error("minimax available without key")
	}
	r.SetAnthropicKey("sk-x")
	if !r.Available("anthropic") {
		t.Error("anthropic not available after key set")
	}
	if r.Available("nope") {
		t.Error("unknown id reported available")
	}
}

// TestGetUnconfiguredReturnsError preserves the historical "not configured"
// errors from each kind's Build.
func TestGetUnconfiguredReturnsError(t *testing.T) {
	r := NewRegistry("") // no keys, force-clear CLI path
	r.claudeCLIPath = ""
	for _, id := range []string{"anthropic", "minimax", "claude-cli"} {
		if _, err := r.Get(id); err == nil {
			t.Errorf("Get(%q): want unconfigured error, got nil", id)
		}
	}
}
