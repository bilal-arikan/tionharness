package conversation

import "testing"

func TestEffectiveBudget(t *testing.T) {
	// Unknown model window → configured value, unchanged.
	if got := EffectiveBudget("openrouter", "openai/gpt-5.5", 12000); got != 12000 {
		t.Fatalf("unknown window: got %d, want 12000 (configured)", got)
	}

	// Configured 0 → default budget as the floor.
	if got := EffectiveBudget("openrouter", "some-unknown", 0); got != defaultMaxTokens {
		t.Fatalf("zero configured, unknown window: got %d, want %d", got, defaultMaxTokens)
	}

	// Haiku 200K × 0.10 = 20000, above the 12000 floor and below the ceil.
	if got, want := EffectiveBudget("anthropic", "claude-haiku-4-5-20251001", 12000), 20000; got != want {
		t.Fatalf("haiku budget: got %d, want %d", got, want)
	}

	// Opus 4.8 is 1M (× 0.10 = 100000) → clamped to the ceil.
	if got := EffectiveBudget("anthropic", "claude-opus-4-8", 12000); got != budgetAutoCeil {
		t.Fatalf("opus budget: got %d, want ceil %d", got, budgetAutoCeil)
	}

	// MiniMax 1M × 0.10 = 100000 → clamped to the ceil.
	if got := EffectiveBudget("minimax", "MiniMax-M3", 12000); got != budgetAutoCeil {
		t.Fatalf("minimax budget: got %d, want ceil %d", got, budgetAutoCeil)
	}

	// Configured floor wins when it exceeds the derived value (Haiku 200K→20000,
	// configured 25000 → 25000).
	if got := EffectiveBudget("anthropic", "claude-haiku-4-5-20251001", 25000); got != 25000 {
		t.Fatalf("high configured floor: got %d, want 25000", got)
	}
}
