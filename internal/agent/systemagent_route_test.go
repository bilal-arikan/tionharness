package agent

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/conversation"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// withAnthropicInstance gives the runtime a registry holding one keyed
// (available) anthropic instance, the precondition for auxiliary routing.
func withAnthropicInstance(rt *Runtime) {
	rt.providers = providers.NewRegistry()
	rt.providers.SetInstances([]providers.Instance{{
		ID: "anthropic-main", KindID: "anthropic", Enabled: true,
		Values: map[string]string{providers.FieldKeyAPIKey: "sk-test"},
	}})
}

// TestRouteAuxAgentCLICallerToAnthropic is the headline case: a claude-cli caller
// with a configured anthropic key runs the auxiliary call on the API instance,
// with the system agent's alias translated to a Messages API id. Identity
// (ID, System, SystemKey) stays on the caller.
func TestRouteAuxAgentCLICallerToAnthropic(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	withAnthropicInstance(rt)

	caller := db.Agent{ID: "A1", Provider: "claude-cli", ProviderInstanceID: "claude-cli", Model: "opus", System: true, SystemKey: "titler"}
	got := rt.routeAuxAgent(caller, db.Agent{Model: "haiku"})
	if got.Provider != "anthropic" || got.ProviderInstanceID != "anthropic-main" {
		t.Fatalf("provider = %q/%q, want anthropic/anthropic-main", got.Provider, got.ProviderInstanceID)
	}
	if got.Model != "claude-haiku-4-5-20251001" {
		t.Fatalf("model = %q, want the haiku API id", got.Model)
	}
	if got.ID != "A1" || !got.System || got.SystemKey != "titler" {
		t.Fatalf("caller identity must be preserved, got %+v", got)
	}
}

// TestRouteAuxAgentStaysPutWithoutKeyOrToggle pins the fallbacks: no anthropic
// instance, tunable off, or a native caller — all leave the copy untouched
// (a native anthropic caller only gets its alias translated).
func TestRouteAuxAgentStaysPutWithoutKeyOrToggle(t *testing.T) {
	rt, tun := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	caller := db.Agent{ID: "A1", Provider: "claude-cli", Model: "opus"}

	// No anthropic instance registered at all.
	if got := rt.routeAuxAgent(caller, db.Agent{Model: "haiku"}); got.Provider != "claude-cli" || got.Model != "opus" {
		t.Fatalf("without an anthropic instance the caller must be unchanged, got %+v", got)
	}

	withAnthropicInstance(rt)
	tun.SetAuxNativeRouting(false)
	if got := rt.routeAuxAgent(caller, db.Agent{Model: "haiku"}); got.Provider != "claude-cli" {
		t.Fatalf("tunable off must keep the CLI caller, got %+v", got)
	}
	tun.SetAuxNativeRouting(true)

	codex := db.Agent{ID: "A2", Provider: "codex-cli", Model: "gpt-5.6-sol"}
	if got := rt.routeAuxAgent(codex, db.Agent{Model: "haiku"}); got.Provider != "anthropic" {
		t.Fatalf("codex-cli is a CLI kind too and must route, got %+v", got)
	}

	native := db.Agent{ID: "A3", Provider: "minimax", Model: "MiniMax-M2"}
	if got := rt.routeAuxAgent(native, db.Agent{Model: "haiku"}); got.Provider != "minimax" || got.Model != "MiniMax-M2" {
		t.Fatalf("a native caller must be unchanged, got %+v", got)
	}

	anth := db.Agent{ID: "A4", Provider: "anthropic", Model: "haiku"}
	if got := rt.routeAuxAgent(anth, db.Agent{Model: "haiku"}); got.Provider != "anthropic" || got.Model != "claude-haiku-4-5-20251001" {
		t.Fatalf("a native anthropic caller keeps its provider but gets the alias translated, got %+v", got)
	}
}

// TestResolveTitleConfigRoutesCLICaller checks routing end-to-end through a
// resolver: the titler prompt still comes from the system agent, the call now
// runs on the anthropic instance.
func TestResolveTitleConfigRoutesCLICaller(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	withAnthropicInstance(rt)
	if err := rt.db.EnsureSystemAgents(context.Background(), SystemAgentDefaults()...); err != nil {
		t.Fatalf("seed system agents: %v", err)
	}

	got, prompt := rt.resolveTitleConfig(db.Agent{ID: "A1", Provider: "claude-cli", Model: "opus"})
	if prompt == "" {
		t.Fatal("title prompt must resolve")
	}
	if got.Provider != "anthropic" || got.Model != "claude-haiku-4-5-20251001" {
		t.Fatalf("title call = %s/%s, want anthropic/haiku API id", got.Provider, got.Model)
	}
}

// TestFoldContextTargets pins the two fold-target shapes: same provider + cheaper
// model stamps an agent-only override (the fold keeps the provider object it was
// handed), while a CLI caller with an anthropic key is routed to the API
// instance; a caller already on the compaction model stamps nothing.
func TestFoldContextTargets(t *testing.T) {
	rt, tun := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()

	// No registry: the fold stays on the handed provider with the compaction
	// agent's model (haiku suggested for a keyless/claude-cli caller).
	fctx := rt.FoldContext(ctx, db.Agent{ID: "A1", Provider: "", Model: "m"})
	target, ok := conversation.FoldTargetAgent(fctx)
	if !ok || target.Model != "haiku" || target.Provider != "" {
		t.Fatalf("same-provider fold target = %+v (ok=%v), want model haiku on the same provider", target, ok)
	}

	// Routed: CLI caller + anthropic key.
	withAnthropicInstance(rt)
	fctx = rt.FoldContext(ctx, db.Agent{ID: "A1", Provider: "claude-cli", Model: "opus"})
	target, ok = conversation.FoldTargetAgent(fctx)
	if !ok || target.Provider != "anthropic" || target.Model != "claude-haiku-4-5-20251001" {
		t.Fatalf("routed fold target = %+v (ok=%v), want anthropic/haiku API id", target, ok)
	}

	// Routing off and caller already on the compaction model: nothing stamped.
	tun.SetAuxNativeRouting(false)
	fctx = rt.FoldContext(ctx, db.Agent{ID: "A1", Provider: "claude-cli", Model: "haiku"})
	if _, ok := conversation.FoldTargetAgent(fctx); ok {
		t.Fatal("no override expected when the caller already matches the fold target")
	}
}

// TestResolveAnalysisSystemAgentHonoursPinnedProvider: when the user pinned a
// provider on a system agent (the "provider" override), the auxiliary call runs
// there — the caller's provider and the anthropic re-routing both yield.
func TestResolveAnalysisSystemAgentHonoursPinnedProvider(t *testing.T) {
	rt, tun := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	withAnthropicInstance(rt)
	tun.SetAuxNativeRouting(true)
	ctx := context.Background()
	parent, err := rt.db.CreateAgent(ctx, db.Agent{
		Name: "Insight Applier", System: true, SystemKey: "insight-applier", Soul: "apply findings",
		Provider: "claude-cli", ProviderInstanceID: "claude-cli", Model: "haiku", Locked: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.db.CreateAgent(ctx, db.Agent{
		Name: "Insight Applier (codex)", System: true, SystemKey: "insight-applier", Soul: "apply findings",
		Provider: "codex-cli", ProviderInstanceID: "codex-cli", Model: "gpt-5.6-sol",
		ParentID: parent.ID, Overrides: []string{"provider", "model"},
	}); err != nil {
		t.Fatal(err)
	}
	caller := db.Agent{ID: "A1", Provider: "claude-cli", ProviderInstanceID: "claude-cli", Model: "opus"}
	got, system, err := rt.resolveAnalysisSystemAgent("insight-applier", caller)
	if err != nil {
		t.Fatal(err)
	}
	if got.Provider != "codex-cli" || got.ProviderInstanceID != "codex-cli" || got.Model != "gpt-5.6-sol" {
		t.Fatalf("pinned provider must win, got %+v", got)
	}
	if system != "apply findings" || got.ID != "A1" || got.SystemKey != "insight-applier" {
		t.Fatalf("prompt / caller identity: %q %+v", system, got)
	}

	// Without the override the historical routing applies (CLI caller → anthropic).
	if _, err := rt.db.CreateAgent(ctx, db.Agent{
		Name: "Goal Writer", System: true, SystemKey: "goal-writer", Soul: "write", Provider: "claude-cli", Model: "sonnet",
	}); err != nil {
		t.Fatal(err)
	}
	got, _, err = rt.resolveAnalysisSystemAgent("goal-writer", caller)
	if err != nil {
		t.Fatal(err)
	}
	if got.Provider != "anthropic" {
		t.Fatalf("unpinned system agent must still route, got %+v", got)
	}
}

// TestAuxRouteQuarantineAfterAuthError: a rejected key takes the anthropic
// instance out of auxiliary routing, so the next call stays on the caller.
func TestAuxRouteQuarantineAfterAuthError(t *testing.T) {
	rt, tun := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	withAnthropicInstance(rt)
	tun.SetAuxNativeRouting(true)
	caller := db.Agent{ID: "A1", Provider: "claude-cli", ProviderInstanceID: "claude-cli", Model: "opus"}
	routed := rt.routeAuxAgent(caller, db.Agent{Model: "haiku"})
	if routed.Provider != "anthropic" {
		t.Fatalf("precondition: expected routing, got %+v", routed)
	}
	rt.noteAuxRouteFailure(routed, errors.New("anthropic API error (authentication_error): invalid x-api-key"))
	if got := rt.routeAuxAgent(caller, db.Agent{Model: "haiku"}); got.Provider != "claude-cli" {
		t.Fatalf("quarantined instance must not be routed to, got %+v", got)
	}
	// Re-saving providers (a new configuration generation) lifts the quarantine.
	rt.providers.SetInstances([]providers.Instance{{
		ID: "anthropic-main", KindID: "anthropic", Enabled: true,
		Values: map[string]string{providers.FieldKeyAPIKey: "sk-fixed"},
	}})
	if got := rt.routeAuxAgent(caller, db.Agent{Model: "haiku"}); got.Provider != "anthropic" {
		t.Fatalf("a re-saved provider config must be tried again, got %+v", got)
	}
	// A non-auth failure does not quarantine.
	rt.auxRouteBad.Delete("anthropic-main")
	rt.noteAuxRouteFailure(routed, errors.New("overloaded_error"))
	if got := rt.routeAuxAgent(caller, db.Agent{Model: "haiku"}); got.Provider != "anthropic" {
		t.Fatalf("a transient error must keep the route, got %+v", got)
	}
}

// TestAuxRouteFallbackRebuildsCaller: after the anthropic route rejects its key,
// the failed call is retried on the caller's own CLI provider (from its row).
func TestAuxRouteFallbackRebuildsCaller(t *testing.T) {
	rt, tun := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	withAnthropicInstance(rt)
	tun.SetAuxNativeRouting(true)
	ctx := context.Background()
	caller, err := rt.db.CreateAgent(ctx, db.Agent{Name: "CEO", Provider: "codex-cli", ProviderInstanceID: "codex-cli", Model: "gpt-5.6-sol"})
	if err != nil {
		t.Fatal(err)
	}
	routed := rt.routeAuxAgent(db.Agent{ID: caller.ID, Provider: caller.Provider, ProviderInstanceID: caller.ProviderInstanceID, Model: caller.Model, System: true, SystemKey: "goal-writer"}, db.Agent{Model: "sonnet"})
	if routed.Provider != "anthropic" {
		t.Fatalf("precondition: expected routing, got %+v", routed)
	}
	authErr := errors.New("anthropic API error (authentication_error): invalid x-api-key")
	fb, ok := rt.auxRouteFallback(ctx, routed, authErr)
	if !ok || fb.Provider != "codex-cli" || fb.ProviderInstanceID != "codex-cli" || fb.Model != "gpt-5.6-sol" {
		t.Fatalf("fallback = %+v ok=%v", fb, ok)
	}
	if fb.ID != caller.ID || !fb.System || fb.SystemKey != "goal-writer" {
		t.Fatalf("identity must survive: %+v", fb)
	}
	// A non-auth error, a non-aux call or a caller that is itself native: no retry.
	if _, ok := rt.auxRouteFallback(ctx, routed, errors.New("overloaded_error")); ok {
		t.Fatal("transient error must not trigger the fallback")
	}
	plain := routed
	plain.SystemKey = ""
	if _, ok := rt.auxRouteFallback(ctx, plain, authErr); ok {
		t.Fatal("a non-aux call must not be re-pointed")
	}
	native, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "API", Provider: "anthropic", ProviderInstanceID: "anthropic-main", Model: "claude-sonnet-5"})
	if _, ok := rt.auxRouteFallback(ctx, db.Agent{ID: native.ID, Provider: "anthropic", ProviderInstanceID: "anthropic-main", SystemKey: "goal-writer"}, authErr); ok {
		t.Fatal("a native caller has no fallback")
	}
}

// TestSystemAgentExecutorUsesOwnProviderAndModel: the system agent's card is
// the rule — its provider/instance/model run the call; the caller only lends
// its id. Routing still applies when the provider was not pinned.
func TestSystemAgentExecutorUsesOwnProviderAndModel(t *testing.T) {
	rt, tun := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	tun.SetAuxNativeRouting(false)
	caller := db.Agent{ID: "A1", Provider: "codex-cli", ProviderInstanceID: "codex-cli", Model: "gpt-5.6-sol"}
	titler := db.Agent{SystemKey: "titler", Provider: "claude-cli", ProviderInstanceID: "claude-cli", Model: "haiku"}
	// The built-in default is only taken when the registry can serve it: a
	// provider with no registered instance keeps the caller's transport.
	unavailable := db.Agent{SystemKey: "titler", Provider: "minimax", ProviderInstanceID: "minimax", Model: "MiniMax-M2"}
	if got := rt.systemAgentExecutor("titler", caller, unavailable); got.Provider != "codex-cli" {
		t.Fatalf("unavailable system provider must keep the caller, got %+v", got)
	}
	rt.providers.SetInstances([]providers.Instance{
		{ID: "anthropic-main", KindID: "anthropic", Enabled: true, Values: map[string]string{providers.FieldKeyAPIKey: "sk-test"}},
		{ID: "claude-cli", KindID: "claude-cli", Enabled: true},
	})
	if !rt.providers.Available("claude-cli") {
		t.Skip("claude-cli not installed on this machine")
	}
	got := rt.systemAgentExecutor("titler", caller, titler)
	if got.Provider != "claude-cli" || got.ProviderInstanceID != "claude-cli" || got.Model != "haiku" {
		t.Fatalf("system agent transport must win: %+v", got)
	}
	if got.ID != "A1" || !got.System || got.SystemKey != "titler" {
		t.Fatalf("caller identity must survive: %+v", got)
	}
	// A system agent without a provider keeps the caller's transport; a claude
	// alias then stays the caller's model on a non-claude provider.
	got = rt.systemAgentExecutor("titler", caller, db.Agent{SystemKey: "titler", Model: "haiku"})
	if got.Provider != "codex-cli" || got.Model != "gpt-5.6-sol" {
		t.Fatalf("no provider on the system agent: caller transport expected, got %+v", got)
	}
	// Unpinned CLI system agent + available anthropic + routing on → routed.
	tun.SetAuxNativeRouting(true)
	if got := rt.systemAgentExecutor("titler", caller, titler); got.Provider != "anthropic" {
		t.Fatalf("unpinned CLI system agent must still route, got %+v", got)
	}
	pinned := titler
	pinned.Overrides = []string{"provider"}
	if got := rt.systemAgentExecutor("titler", caller, pinned); got.Provider != "claude-cli" {
		t.Fatalf("pinned provider must not route, got %+v", got)
	}
}
