package providers

import (
	"context"
	"testing"
	"time"
)

// TestLongRequestModel pins which classes get the long per-request budget:
// adaptive-thinking models AND reasoning models on OpenAI-compatible endpoints
// (DeepSeek V4 Pro), but not the non-reasoning flash tier.
func TestLongRequestModel(t *testing.T) {
	long := []string{"claude-fable-5", "claude-opus-4-8", "claude-sonnet-5", "deepseek-v4-pro", "deepseek-reasoner"}
	for _, m := range long {
		if !LongRequestModel(m) {
			t.Errorf("%q should use the long budget", m)
		}
	}
	short := []string{"claude-haiku-4-5", "deepseek-v4-flash", "MiniMax-M2.1", "gpt-4o-mini"}
	for _, m := range short {
		if LongRequestModel(m) {
			t.Errorf("%q should keep the short budget", m)
		}
	}
}

// TestOpenAICompatRequestCtx pins that the OpenAI-compatible client is now
// model-class aware: DeepSeek V4 Pro gets minutes, the flash tier keeps 120s.
// This is the direct fix for the SES446 "context deadline exceeded" at 120s.
func TestOpenAICompatRequestCtx(t *testing.T) {
	m := NewOpenAICompat("deepseek", "k", "", "deepseek-v4-flash")

	ctxPro, cancel := m.requestCtx(context.Background(), "deepseek-v4-pro")
	defer cancel()
	if dl, ok := ctxPro.Deadline(); !ok || time.Until(dl) < 9*time.Minute {
		t.Errorf("deepseek-v4-pro should get the long budget, got %v", time.Until(dl))
	}

	ctxFlash, cancel2 := m.requestCtx(context.Background(), "deepseek-v4-flash")
	defer cancel2()
	if dl, _ := ctxFlash.Deadline(); time.Until(dl) > 3*time.Minute {
		t.Errorf("deepseek-v4-flash should keep the short budget, got %v", time.Until(dl))
	}
}

// TestRequestTimeoutOverride pins the manifest-driven override: a positive
// setRequestTimeout wins over the model-class default and lifts the http.Client
// safety net above the new budget.
func TestRequestTimeoutOverride(t *testing.T) {
	m := NewOpenAICompat("x", "k", "", "some-model")
	var tc requestTimeoutConfigurable = m
	tc.setRequestTimeout(1800) // 30 min

	ctx, cancel := m.requestCtx(context.Background(), "some-model") // non-reasoning → would be 120s
	defer cancel()
	if dl, ok := ctx.Deadline(); !ok || time.Until(dl) < 29*time.Minute {
		t.Errorf("override should win, got %v", time.Until(dl))
	}
	if m.client.Timeout < 1800*time.Second {
		t.Errorf("client safety net should be lifted above the budget, got %v", m.client.Timeout)
	}
}
