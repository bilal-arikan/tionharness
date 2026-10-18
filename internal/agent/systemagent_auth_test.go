package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

func withNativeInstance(rt *Runtime) {
	rt.providers = providers.NewRegistry()
	rt.providers.SetInstances([]providers.Instance{{
		ID: "native-main", KindID: "claude-cli", Enabled: true,
		Values: map[string]string{providers.FieldKeyCLIPath: "/usr/bin/claude"},
	}})
}

func TestUnpinnedSystemAuthFallsBackAndQuarantines(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	withNativeInstance(rt)
	ctx := context.Background()
	caller, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Caller", Provider: "claude-cli", ProviderInstanceID: "native-main", Model: "claude-sonnet-4-6"})
	if err != nil {
		t.Fatal(err)
	}
	routed := caller
	routed.System = true
	routed.SystemKey = "flow-optimizer"
	routed.Provider = "claude-cli"
	routed.ProviderInstanceID = "claude-cli"
	authErr := errors.New("claude CLI authentication failed: organization disabled subscription access")
	fallback, ok := rt.systemAuthFallback(ctx, routed, authErr)
	if !ok || fallback.ProviderRef() != caller.ProviderRef() || fallback.ID != caller.ID || fallback.SystemKey != "flow-optimizer" {
		t.Fatalf("fallback: %+v / %v", fallback, ok)
	}
	if !rt.systemRouteQuarantined("claude-cli") {
		t.Fatal("rejected default was not quarantined")
	}
	rt.providers.SetInstances([]providers.Instance{{ID: "native-main", KindID: "claude-cli", Enabled: true, Values: map[string]string{providers.FieldKeyCLIPath: "/usr/local/bin/claude"}}})
	if rt.systemRouteQuarantined("claude-cli") {
		t.Fatal("provider changes did not clear quarantine")
	}
	if _, ok := rt.systemAuthFallback(ctx, routed, errors.New("request timed out")); ok {
		t.Fatal("non-auth failures changed transport")
	}
	if _, ok := rt.systemAuthFallback(ctx, caller, authErr); ok {
		t.Fatal("ordinary agent call changed transport")
	}
}

func TestPinnedSystemAuthNeverChangesProvider(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	withNativeInstance(rt)
	ctx := context.Background()
	caller, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Caller", Provider: "claude-cli", ProviderInstanceID: "native-main"})
	if err != nil {
		t.Fatal(err)
	}
	parent, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Default judge", System: true, SystemKey: "flow-optimizer", Provider: "claude-cli", Locked: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Pinned judge", System: true, SystemKey: "flow-optimizer", Provider: "claude-cli", ParentID: parent.ID, Overrides: []string{"provider"}}); err != nil {
		t.Fatal(err)
	}
	routed := caller
	routed.SystemKey = "flow-optimizer"
	routed.Provider = "claude-cli"
	routed.ProviderInstanceID = "claude-cli"
	if _, ok := rt.systemAuthFallback(ctx, routed, errors.New("authentication failed")); ok {
		t.Fatal("explicit provider pin was ignored")
	}
}
