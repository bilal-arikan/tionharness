package conversation

import "testing"

// c is the package default ceil (262144). Explicit-fraction tests pass 0.6 so the
// "fixed manual share" path is exercised; the adaptive (fraction<=0) path is tested
// separately in TestEffectiveBudgetAdaptive.
const c = defaultBudgetAutoCeil

func TestEffectiveBudget(t *testing.T) {
	// Unknown model window → configured value, unchanged (fraction irrelevant).
	if got := EffectiveBudget("openrouter", "openai/gpt-5.5", 12000, 0.6, c); got != 12000 {
		t.Fatalf("unknown window: got %d, want 12000 (configured)", got)
	}

	// Configured 0 → default budget as the floor.
	if got := EffectiveBudget("openrouter", "some-unknown", 0, 0.6, c); got != defaultMaxTokens {
		t.Fatalf("zero configured, unknown window: got %d, want %d", got, defaultMaxTokens)
	}

	// Explicit fraction wins over the adaptive table: Haiku 200K × 0.6 = 120000,
	// above the 12000 floor and below the ceil.
	if got, want := EffectiveBudget("anthropic", "claude-haiku-4-5-20251001", 12000, 0.6, c), 120000; got != want {
		t.Fatalf("haiku explicit budget: got %d, want %d", got, want)
	}

	// Explicit fraction: Opus 4.8 is 1M (× 0.6 = 600000) → clamped to the ceil.
	if got := EffectiveBudget("anthropic", "claude-opus-4-8", 12000, 0.6, c); got != defaultBudgetAutoCeil {
		t.Fatalf("opus explicit budget: got %d, want ceil %d", got, defaultBudgetAutoCeil)
	}

	// Configured floor wins when it exceeds the derived value (Haiku 200K×0.6=120000,
	// configured 200000 → 200000).
	if got := EffectiveBudget("anthropic", "claude-haiku-4-5-20251001", 200000, 0.6, c); got != 200000 {
		t.Fatalf("high configured floor: got %d, want 200000", got)
	}
}

func TestEffectiveBudgetAdaptive(t *testing.T) {
	// fraction<=0 → per-family adaptive share (context-rot aware).
	// Haiku adaptive 0.40 × 200K = 80000 (below ceil, above floor).
	if got, want := EffectiveBudget("anthropic", "claude-haiku-4-5-20251001", 12000, 0, 0), 80000; got != want {
		t.Fatalf("haiku adaptive: got %d, want %d", got, want)
	}

	// Opus adaptive 0.45 × 1M = 450000 → clamped to the ceil (262144).
	if got := EffectiveBudget("anthropic", "claude-opus-4-8", 12000, 0, 0); got != defaultBudgetAutoCeil {
		t.Fatalf("opus adaptive: got %d, want ceil %d", got, defaultBudgetAutoCeil)
	}

	// MiniMax adaptive 0.35 × 1M = 350000 → clamped to the ceil.
	if got := EffectiveBudget("minimax", "MiniMax-M3", 12000, 0, 0); got != defaultBudgetAutoCeil {
		t.Fatalf("minimax adaptive: got %d, want ceil %d", got, defaultBudgetAutoCeil)
	}

	// Unknown family with auto fraction: window is unknown (0) → configured floor,
	// the adaptive table never matters.
	if got := EffectiveBudget("openrouter", "openai/gpt-5.5", 12000, 0, 0); got != 12000 {
		t.Fatalf("unknown adaptive: got %d, want 12000 (configured)", got)
	}
}
