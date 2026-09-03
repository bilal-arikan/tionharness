package agent

import (
	"context"
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
