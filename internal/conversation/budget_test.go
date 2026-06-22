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

	// Claude 200K × 0.10 = 20000, above the 12000 floor and below the ceil.
	if got, want := EffectiveBudget("anthropic", "claude-opus-4-8", 12000), 20000; got != want {
		t.Fatalf("claude budget: got %d, want %d", got, want)
	}

	// MiniMax 1M × 0.10 = 100000 → clamped to the ceil.
	if got := EffectiveBudget("minimax", "MiniMax-M3", 12000); got != budgetAutoCeil {
		t.Fatalf("minimax budget: got %d, want ceil %d", got, budgetAutoCeil)
	}

	// Configured floor always wins when it exceeds the derived value.
	if got := EffectiveBudget("anthropic", "claude-opus-4-8", 30000); got != 30000 {
		t.Fatalf("high configured floor: got %d, want 30000", got)
	}
}
