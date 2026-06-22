package agent

import "testing"

// TestCompactThresholdsScaleWithBudget covers CG-9 (second half): tool-output
// byte thresholds scale with the context budget, the A-cap stays above the
// B-threshold at every scale, and the default budget reproduces the configured
// defaults exactly.
func TestCompactThresholdsScaleWithBudget(t *testing.T) {
	tun := NewTunables()
	// Configure the shipped defaults explicitly (0 would also select them).
	tun.SetToolCompaction(true, 200, 16384, true, 12288, "")

	// Default budget → scale 1.0 → exact configured values.
	tun.SetContextBudget(defaultContextBudgetTokens)
	if got := tun.CompactMaxBytes(); got != 16384 {
		t.Fatalf("default budget A-cap: got %d, want 16384", got)
	}
	if got := tun.CompactLLMThreshold(); got != 12288 {
		t.Fatalf("default budget B-threshold: got %d, want 12288", got)
	}

	// Zero budget behaves as the default (scale 1).
	tun.SetContextBudget(0)
	if got := tun.CompactLLMThreshold(); got != 12288 {
		t.Fatalf("zero budget B-threshold: got %d, want 12288", got)
	}

	// 3× budget → 3× thresholds, invariant A-cap > B-threshold holds.
	tun.SetContextBudget(3 * defaultContextBudgetTokens)
	if got, want := tun.CompactMaxBytes(), 3*16384; got != want {
		t.Fatalf("3x A-cap: got %d, want %d", got, want)
	}
	if got, want := tun.CompactLLMThreshold(), 3*12288; got != want {
		t.Fatalf("3x B-threshold: got %d, want %d", got, want)
	}
	if tun.CompactMaxBytes() <= tun.CompactLLMThreshold() {
		t.Fatalf("invariant broken at 3x: A-cap %d must exceed B-threshold %d",
			tun.CompactMaxBytes(), tun.CompactLLMThreshold())
	}

	// 10× budget → clamped to 5×, never explodes.
	tun.SetContextBudget(10 * defaultContextBudgetTokens)
	if got, want := tun.CompactLLMThreshold(), 5*12288; got != want {
		t.Fatalf("clamp at 5x: got %d, want %d", got, want)
	}

	// Small budget never shrinks below the configured value (scale floored at 1).
	tun.SetContextBudget(defaultContextBudgetTokens / 4)
	if got := tun.CompactLLMThreshold(); got != 12288 {
		t.Fatalf("small budget should floor at configured: got %d, want 12288", got)
	}
}
