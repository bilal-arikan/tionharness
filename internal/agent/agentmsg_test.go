package agent

import (
	"testing"
)

func TestResolveThinkingBudget(t *testing.T) {
	// Classic models: unchanged (off stays off, levels map as before).
	if got := resolveThinkingBudget("claude-opus-4-8", "off"); got != 0 {
		t.Errorf("classic off = %d, want 0", got)
	}
	if got := resolveThinkingBudget("claude-opus-4-8", "high"); got != 16384 {
		t.Errorf("classic high = %d, want 16384", got)
	}
	// Adaptive-only models: off is floored to the minimal adaptive budget.
	if got := resolveThinkingBudget("claude-fable-5", "off"); got != 1024 {
		t.Errorf("fable off = %d, want 1024 (min adaptive)", got)
	}
	// Adaptive model with an explicit higher level keeps the higher budget.
	if got := resolveThinkingBudget("claude-fable-5", "high"); got != 16384 {
		t.Errorf("fable high = %d, want 16384", got)
	}
}
