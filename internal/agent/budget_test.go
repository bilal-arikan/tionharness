package agent

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/conversation"
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
	a := providers.Usage{InputTokens: 100, OutputTokens: 10, CacheReadTokens: 1000, CacheWriteTokens: 50, ThinkingTokens: 3}
	b := providers.Usage{InputTokens: 200, OutputTokens: 20, CacheReadTokens: 2000, CacheWriteTokens: 60, ThinkingTokens: 7}
	got := sumUsage(a, b)
	want := providers.Usage{InputTokens: 300, OutputTokens: 30, CacheReadTokens: 3000, CacheWriteTokens: 110, ThinkingTokens: 10}
	if got != want {
		t.Fatalf("sumUsage = %+v, want %+v", got, want)
	}

	// Zero value is the identity element (loop starts from an empty accumulator).
	if got := sumUsage(providers.Usage{}, b); got != b {
		t.Fatalf("sumUsage(zero, b) = %+v, want %+v", got, b)
	}
}

// TestDeriveThinkingTokens checks the hidden-reasoning derivation: thinking is
// OutputTokens minus the estimated visible payload, clamped at 0, and disabled
// for claude-cli's cumulative multi-step turns (ProviderCalls > 1).
func TestDeriveThinkingTokens(t *testing.T) {
	txt := strings.Repeat("word ", 100) // whitespace-rich prose
	vis := conversation.EstimateText(txt)

	// Output well above the visible estimate → the excess is attributed to thinking.
	resp := &providers.Response{Text: txt, Usage: providers.Usage{OutputTokens: vis + 400}}
	if got := deriveThinkingTokens(resp); got != 400 {
		t.Fatalf("thinking = %d, want 400", got)
	}

	// Visible estimate exceeds output → clamp to 0, never negative.
	resp2 := &providers.Response{Text: txt, Usage: providers.Usage{OutputTokens: vis - 5}}
	if got := deriveThinkingTokens(resp2); got != 0 {
		t.Fatalf("clamp = %d, want 0", got)
	}

	// claude-cli cumulative turn (ProviderCalls > 1): subtraction is meaningless → 0.
	resp3 := &providers.Response{Text: txt, Usage: providers.Usage{OutputTokens: 9999}, ProviderCalls: 3}
	if got := deriveThinkingTokens(resp3); got != 0 {
		t.Fatalf("cli-cumulative = %d, want 0", got)
	}

	// nil is safe.
	if got := deriveThinkingTokens(nil); got != 0 {
		t.Fatalf("nil = %d, want 0", got)
	}
}
