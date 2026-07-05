package agent

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
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

// TestSumUsage checks the native tool loop's per-turn accumulator adds every
// counter field, so a multi-step turn's persisted bubble equals the sum of its
// provider calls (matching what RecordUsage folds into the daily/session rollups).
func TestSumUsage(t *testing.T) {
	a := providers.Usage{InputTokens: 100, OutputTokens: 10, CacheReadTokens: 1000, CacheWriteTokens: 50}
	b := providers.Usage{InputTokens: 200, OutputTokens: 20, CacheReadTokens: 2000, CacheWriteTokens: 60}
	got := sumUsage(a, b)
	want := providers.Usage{InputTokens: 300, OutputTokens: 30, CacheReadTokens: 3000, CacheWriteTokens: 110}
	if got != want {
		t.Fatalf("sumUsage = %+v, want %+v", got, want)
	}

	// Zero value is the identity element (loop starts from an empty accumulator).
	if got := sumUsage(providers.Usage{}, b); got != b {
		t.Fatalf("sumUsage(zero, b) = %+v, want %+v", got, b)
	}
}
