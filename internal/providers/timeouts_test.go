package providers

import (
	"context"
	"testing"
	"time"
)

// TestLongRequestModel pins which classes get the long per-request budget:
// adaptive-thinking models AND reasoning models on OpenAI-compatible endpoints
// (the DeepSeek V4 family incl. V4.1 Flash, the GLM-5.3 family), but not
// non-reasoning or legacy models.
func TestLongRequestModel(t *testing.T) {
	long := []string{
		"claude-fable-5", "claude-opus-4-8", "claude-sonnet-5", "deepseek-v4-pro", "deepseek-reasoner",
		"deepseek-flash", "deepseek-v4-flash", "glm-5.3", "glm-5.3-flash",
	}
	for _, m := range long {
		if !LongRequestModel(m) {
			t.Errorf("%q should use the long budget", m)
		}
	}
	short := []string{"claude-haiku-4-5", "MiniMax-M2.1", "gpt-4o-mini", "glm-5.2"}
	for _, m := range short {
		if LongRequestModel(m) {
			t.Errorf("%q should keep the short budget", m)
		}
	}
}

// TestOpenAICompatRequestCtx pins that the OpenAI-compatible client is
// model-class aware: DeepSeek's reasoning models get minutes, a non-reasoning
// model keeps 120s. This is the direct fix for the SES446 "context deadline
// exceeded" at 120s.
func TestOpenAICompatRequestCtx(t *testing.T) {
	m := NewOpenAICompat("deepseek", "k", "", "deepseek-flash")

	for _, model := range []string{"deepseek-v4-pro", "deepseek-flash"} {
		ctx, cancel := m.requestCtx(context.Background(), model)
		if dl, ok := ctx.Deadline(); !ok || time.Until(dl) < 9*time.Minute {
			t.Errorf("%s should get the long budget, got %v", model, time.Until(dl))
		}
		cancel()
	}

	ctxShort, cancel2 := m.requestCtx(context.Background(), "MiniMax-M2.1")
	defer cancel2()
	if dl, _ := ctxShort.Deadline(); time.Until(dl) > 3*time.Minute {
		t.Errorf("MiniMax-M2.1 should keep the short budget, got %v", time.Until(dl))
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
