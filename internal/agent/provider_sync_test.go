package agent

import (
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// TestSyncProviderFieldsInvariant verifies the core Faz 2 invariant
// (_Docs/71 §2.5, K3): after SyncProviderFields resolves an instance id, the
// returned provider ALWAYS equals Registry.KindOf(providerInstanceID) — the
// property every agent-writing call site (agents.go, builtin_agentmgmt.go,
// market_install.go, templates.go, coordination.go) relies on.
func TestSyncProviderFieldsInvariant(t *testing.T) {
	r := providers.NewRegistry()
	r.SetInstances([]providers.Instance{
		{ID: "anthropic", KindID: "anthropic"},
		{ID: "claude-cli", KindID: "claude-cli"},
		{ID: "PRV3", KindID: "anthropic"}, // a second anthropic instance, non-default id
	})

	cases := []struct {
		name       string
		instanceID string
		wantKind   string
		wantErr    bool
	}{
		{"default anthropic instance", "anthropic", "anthropic", false},
		{"default claude-cli instance", "claude-cli", "claude-cli", false},
		{"non-default instance of a shared kind", "PRV3", "anthropic", false},
		{"empty resolves to keyless claude-cli default", "", "claude-cli", false},
		{"unknown/deleted instance errors", "PRV999", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			provider, providerInstanceID, err := SyncProviderFields(r, c.instanceID)
			if c.wantErr {
				if err == nil {
					t.Fatalf("SyncProviderFields(%q): want error, got nil", c.instanceID)
				}
				return
			}
			if err != nil {
				t.Fatalf("SyncProviderFields(%q): %v", c.instanceID, err)
			}
			if provider != c.wantKind {
				t.Errorf("provider = %q, want %q", provider, c.wantKind)
			}
			// The core invariant: Provider must always equal KindOf(ProviderInstanceID).
			if got := r.KindOf(providerInstanceID); got != provider {
				t.Errorf("invariant broken: KindOf(%q) = %q, want it to equal provider %q", providerInstanceID, got, provider)
			}
		})
	}
}
