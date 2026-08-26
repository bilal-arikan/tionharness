package agent

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func newSystemAgentResolveRuntime(t *testing.T) *Runtime {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return &Runtime{
		db:     database,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func TestResolveSystemAgentEnabledWorkspaceAgent(t *testing.T) {
	rt := newSystemAgentResolveRuntime(t)
	want, err := rt.db.CreateAgent(context.Background(), db.Agent{
		Name: "Custom Titler", System: true, SystemKey: "titler", Model: "custom-model",
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	got, usedFallback, err := rt.ResolveSystemAgent("titler")
	if err != nil {
		t.Fatalf("ResolveSystemAgent: %v", err)
	}
	if usedFallback {
		t.Fatal("ResolveSystemAgent used fallback for enabled workspace agent")
	}
	if got.ID != want.ID || got.Name != want.Name || got.Model != want.Model {
		t.Fatalf("ResolveSystemAgent = %+v, want workspace agent %+v", got, want)
	}
}

func TestResolveSystemAgentDisabledWorkspaceAgentFallsBack(t *testing.T) {
	rt := newSystemAgentResolveRuntime(t)
	if _, err := rt.db.CreateAgent(context.Background(), db.Agent{
		Name: "Disabled Titler", System: true, SystemKey: "titler", Disabled: true,
	}); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	got, usedFallback, err := rt.ResolveSystemAgent("titler")
	if err != nil {
		t.Fatalf("ResolveSystemAgent: %v", err)
	}
	assertSystemAgentFallback(t, got, usedFallback, "titler")
}

func TestResolveSystemAgentMissingWorkspaceAgentFallsBack(t *testing.T) {
	rt := newSystemAgentResolveRuntime(t)

	got, usedFallback, err := rt.ResolveSystemAgent("titler")
	if err != nil {
		t.Fatalf("ResolveSystemAgent: %v", err)
	}
	assertSystemAgentFallback(t, got, usedFallback, "titler")
}

func TestResolveSystemAgentUnknownKeyReturnsError(t *testing.T) {
	rt := newSystemAgentResolveRuntime(t)

	_, _, err := rt.ResolveSystemAgent("unknown")
	if err == nil {
		t.Fatal("ResolveSystemAgent returned nil error for unknown key")
	}
}

func TestSystemAgentDefaultsHavePromptAndModel(t *testing.T) {
	for _, def := range SystemAgentDefaults() {
		if def.SystemPrompt == "" {
			t.Errorf("system agent default %q has empty SystemPrompt", def.SystemKey)
		}
		if def.SuggestedModel == "" {
			t.Errorf("system agent default %q has empty SuggestedModel", def.SystemKey)
		}
	}
}

func assertSystemAgentFallback(t *testing.T, got db.Agent, usedFallback bool, key string) {
	t.Helper()
	if !usedFallback {
		t.Fatal("ResolveSystemAgent did not report built-in fallback")
	}
	want, ok := SystemAgentDefault(key)
	if !ok {
		t.Fatalf("missing system agent default %q", key)
	}
	if got.Soul == "" {
		t.Fatalf("ResolveSystemAgent fallback %q has empty system prompt", key)
	}
	if got.Model == "" {
		t.Fatalf("ResolveSystemAgent fallback %q has empty model", key)
	}
	if got.ID != "" || got.SystemKey != want.SystemKey || got.Name != want.Name ||
		got.Identity != want.Description || got.Soul != want.SystemPrompt ||
		got.Model != want.SuggestedModel || got.AllowedTools != want.AllowedTools {
		t.Fatalf("ResolveSystemAgent fallback = %+v, want definition %+v", got, want)
	}
}
