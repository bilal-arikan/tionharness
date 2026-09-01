package agent

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/conversation"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

type measuredThinkingProvider struct{}

func (measuredThinkingProvider) Name() string { return "measured-thinking-test" }
func (measuredThinkingProvider) Complete(context.Context, providers.Request) (*providers.Response, error) {
	return measuredThinkingResponse(), nil
}
func (measuredThinkingProvider) Stream(_ context.Context, _ providers.Request, onDelta func(providers.StreamDelta)) (*providers.Response, error) {
	onDelta(providers.StreamDelta{Kind: providers.DeltaText, Text: "OK"})
	return measuredThinkingResponse(), nil
}

func measuredThinkingResponse() *providers.Response {
	return &providers.Response{
		Text:          "OK",
		Model:         "test-model",
		Usage:         providers.Usage{OutputTokens: 100, ThinkingTokens: 33, ThinkingTokensMeasured: true},
		ProviderCalls: 3,
	}
}

type fixedThinkingProvider struct {
	usage providers.Usage
}

func (fixedThinkingProvider) Name() string { return "fixed-thinking-test" }
func (p fixedThinkingProvider) Complete(context.Context, providers.Request) (*providers.Response, error) {
	return &providers.Response{Text: strings.Repeat("word ", 20), Model: "test-model", Usage: p.usage}, nil
}
func (p fixedThinkingProvider) Stream(_ context.Context, _ providers.Request, onDelta func(providers.StreamDelta)) (*providers.Response, error) {
	onDelta(providers.StreamDelta{Kind: providers.DeltaText, Text: "OK"})
	return p.Complete(context.Background(), providers.Request{})
}

var registerMeasuredThinkingProvider sync.Once

func configureMeasuredThinkingProvider(rt *Runtime) {
	registerMeasuredThinkingProvider.Do(func() {
		providers.RegisterKind(providers.NewBuiltinKind(
			providers.Manifest{Kind: "measured-thinking-test", Transport: providers.TransportAPI},
			func(providers.ResolvedConfig) bool { return true },
			func(providers.ResolvedConfig) (providers.Provider, error) { return measuredThinkingProvider{}, nil },
		))
	})
	rt.providers.SetInstances([]providers.Instance{{ID: "measured-thinking-test", KindID: "measured-thinking-test"}})
}

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

func TestPreserveOrDeriveThinkingTokens(t *testing.T) {
	measured := &providers.Response{
		Text:          "OK",
		Usage:         providers.Usage{OutputTokens: 100, ThinkingTokens: 33, ThinkingTokensMeasured: true},
		ProviderCalls: 3,
	}
	preserveOrDeriveThinkingTokens(measured)
	if measured.Usage.ThinkingTokens != 33 {
		t.Fatalf("measured thinking = %d, want 33", measured.Usage.ThinkingTokens)
	}
	measuredZero := &providers.Response{
		Text:  strings.Repeat("word ", 20),
		Usage: providers.Usage{OutputTokens: 100, ThinkingTokensMeasured: true},
	}
	preserveOrDeriveThinkingTokens(measuredZero)
	if measuredZero.Usage.ThinkingTokens != 0 {
		t.Fatalf("measured zero thinking = %d, want 0", measuredZero.Usage.ThinkingTokens)
	}

	text := strings.Repeat("word ", 20)
	visible := conversation.EstimateText(text)
	estimated := &providers.Response{Text: text, Usage: providers.Usage{OutputTokens: visible + 17}}
	preserveOrDeriveThinkingTokens(estimated)
	if estimated.Usage.ThinkingTokens != 17 {
		t.Fatalf("estimated thinking = %d, want 17", estimated.Usage.ThinkingTokens)
	}
}

func TestMeasuredThinkingSurvivesCompletionStreamingAndGuardedPaths(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	agent := db.Agent{ID: "thinking-agent", Provider: "measured-thinking-test", Model: "test-model"}
	assertMeasured := func(path string, resp *providers.Response, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if resp.Usage.ThinkingTokens != 33 {
			t.Fatalf("%s thinking = %d, want 33", path, resp.Usage.ThinkingTokens)
		}
	}

	resp, err := rt.recordedComplete(context.Background(), agent, measuredThinkingProvider{}, providers.Request{})
	assertMeasured("completion", resp, err)
	resp, err = rt.recordedStream(context.Background(), agent, measuredThinkingProvider{}, providers.Request{}, func(TurnStep) {})
	assertMeasured("streaming", resp, err)

	configureMeasuredThinkingProvider(rt)
	resp, err = rt.guardedComplete(context.Background(), agent, providers.Request{}, false)
	assertMeasured("guarded", resp, err)
}

func TestMeasuredZeroAndAbsentThinkingAcrossCompletionAndStreaming(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	agent := db.Agent{ID: "thinking-agent", Provider: "fixed-thinking-test", Model: "test-model"}
	for _, tc := range []struct {
		name  string
		usage providers.Usage
		want  int
	}{
		{"measured-zero", providers.Usage{OutputTokens: 100, ThinkingTokensMeasured: true}, 0},
		{"unmeasured", providers.Usage{OutputTokens: 100}, 100 - conversation.EstimateText(strings.Repeat("word ", 20))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := fixedThinkingProvider{usage: tc.usage}
			resp, err := rt.recordedComplete(context.Background(), agent, provider, providers.Request{})
			if err != nil || resp.Usage.ThinkingTokens != tc.want {
				t.Fatalf("completion: thinking=%d err=%v, want %d", resp.Usage.ThinkingTokens, err, tc.want)
			}
			resp, err = rt.recordedStream(context.Background(), agent, provider, providers.Request{}, func(TurnStep) {})
			if err != nil || resp.Usage.ThinkingTokens != tc.want {
				t.Fatalf("streaming: thinking=%d err=%v, want %d", resp.Usage.ThinkingTokens, err, tc.want)
			}
		})
	}
}
