package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

func TestUnpinnedSystemAuthFallsBackAndQuarantines(t *testing.T) {
	rt, tun := newTestRuntime(t, t.TempDir())
	withAnthropicInstance(rt)
	tun.SetAuxNativeRouting(false)
	ctx := context.Background()
	caller, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Caller", Provider: "anthropic", ProviderInstanceID: "anthropic-main", Model: "claude-sonnet-4-6"})
	if err != nil {
		t.Fatal(err)
	}
	routed := caller
	routed.System = true
	routed.SystemKey = "stall-judge"
	routed.Provider = "claude-cli"
	routed.ProviderInstanceID = "claude-cli"
	authErr := errors.New("claude CLI authentication failed: organization disabled subscription access")
	fallback, ok := rt.systemAuthFallback(ctx, routed, authErr)
	if !ok || fallback.ProviderRef() != caller.ProviderRef() || fallback.ID != caller.ID || fallback.SystemKey != "stall-judge" {
		t.Fatalf("fallback: %+v / %v", fallback, ok)
	}
	if !rt.systemRouteQuarantined("claude-cli") {
		t.Fatal("rejected default was not quarantined")
	}
	rt.providers.SetInstances([]providers.Instance{{ID: "anthropic-main", KindID: "anthropic", Enabled: true, Values: map[string]string{providers.FieldKeyAPIKey: "updated-key"}}})
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
	withAnthropicInstance(rt)
	ctx := context.Background()
	caller, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Caller", Provider: "anthropic", ProviderInstanceID: "anthropic-main"})
	if err != nil {
		t.Fatal(err)
	}
	parent, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Default judge", System: true, SystemKey: "stall-judge", Provider: "claude-cli", Locked: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Pinned judge", System: true, SystemKey: "stall-judge", Provider: "claude-cli", ParentID: parent.ID, Overrides: []string{"provider"}}); err != nil {
		t.Fatal(err)
	}
	routed := caller
	routed.SystemKey = "stall-judge"
	routed.Provider = "claude-cli"
	routed.ProviderInstanceID = "claude-cli"
	if _, ok := rt.systemAuthFallback(ctx, routed, errors.New("authentication failed")); ok {
		t.Fatal("explicit provider pin was ignored")
	}
}
