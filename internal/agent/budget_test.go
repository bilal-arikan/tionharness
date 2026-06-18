package agent

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/providers"
)

// TestGuardedComplete_LogsProviderResolveFailure guards the logging-consistency
// fix: the utility funnel used by reflect/summary/title must record a provider
// failure, not just propagate it silently. An anthropic agent with no key fails
// provider resolution, which must surface as a Warn record carrying the call
// origin so the logs view explains a failed background reflect/title/summary.
func TestGuardedComplete_LogsProviderResolveFailure(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	var cap capturingHandler
	rt.logger = slog.New(&cap)
	ctx := WithCallKind(context.Background(), KindReflect)

	agent, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Düşünür", Provider: "anthropic", Model: "m"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	_, err = rt.guardedComplete(ctx, agent, providers.Request{Model: "m"}, false)
	if err == nil {
		t.Fatal("expected guardedComplete to fail with unconfigured provider")
	}

	var found bool
	for _, r := range cap.records() {
		if r.Level == slog.LevelWarn && r.Message == "provider resolve failed" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a Warn 'provider resolve failed' record, got %+v", cap.records())
	}
}
