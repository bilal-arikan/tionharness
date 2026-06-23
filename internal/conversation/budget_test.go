package conversation

import "testing"

// f, c are the package default knobs (fraction 0.6, ceil 512000) used throughout.
const (
	f = defaultBudgetWindowFraction
	c = defaultBudgetAutoCeil
)

func TestEffectiveBudget(t *testing.T) {
	// Unknown model window → configured value, unchanged.
	if got := EffectiveBudget("openrouter", "openai/gpt-5.5", 12000, f, c); got != 12000 {
		t.Fatalf("unknown window: got %d, want 12000 (configured)", got)
	}

	// Configured 0 → default budget as the floor.
	if got := EffectiveBudget("openrouter", "some-unknown", 0, f, c); got != defaultMaxTokens {
		t.Fatalf("zero configured, unknown window: got %d, want %d", got, defaultMaxTokens)
	}

	// Haiku 200K × 0.6 = 120000, above the 12000 floor and below the ceil.
	if got, want := EffectiveBudget("anthropic", "claude-haiku-4-5-20251001", 12000, f, c), 120000; got != want {
		t.Fatalf("haiku budget: got %d, want %d", got, want)
	}

	// Opus 4.8 is 1M (× 0.6 = 600000) → clamped to the ceil (512000).
	if got := EffectiveBudget("anthropic", "claude-opus-4-8", 12000, f, c); got != defaultBudgetAutoCeil {
		t.Fatalf("opus budget: got %d, want ceil %d", got, defaultBudgetAutoCeil)
	}

	// MiniMax 1M × 0.6 = 600000 → clamped to the ceil.
	if got := EffectiveBudget("minimax", "MiniMax-M3", 12000, f, c); got != defaultBudgetAutoCeil {
		t.Fatalf("minimax budget: got %d, want ceil %d", got, defaultBudgetAutoCeil)
	}

	// Non-positive fraction/ceil fall back to the package defaults (Haiku → 120000).
	if got := EffectiveBudget("anthropic", "claude-haiku-4-5-20251001", 12000, 0, 0); got != 120000 {
		t.Fatalf("zero knobs fallback: got %d, want 120000", got)
	}

	// Configured floor wins when it exceeds the derived value (Haiku 200K→120000,
	// configured 200000 → 200000).
	if got := EffectiveBudget("anthropic", "claude-haiku-4-5-20251001", 200000, f, c); got != 200000 {
		t.Fatalf("high configured floor: got %d, want 200000", got)
	}
}
