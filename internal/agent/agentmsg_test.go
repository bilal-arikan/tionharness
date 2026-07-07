package agent

import (
	"testing"
)

func TestResolveThinkingBudget(t *testing.T) {
	// Levels map to budgets uniformly; model-class wire translation (adaptive
	// vs legacy, always-on) happens in the provider layer.
	if got := resolveThinkingBudget("claude-opus-4-8", "off"); got != 0 {
		t.Errorf("off = %d, want 0", got)
	}
	if got := resolveThinkingBudget("claude-opus-4-8", "high"); got != 16384 {
		t.Errorf("high = %d, want 16384", got)
	}
	// Always-on models no longer get a floored budget: "off" passes through as
	// 0 and the provider omits the thinking field (server thinks anyway).
	if got := resolveThinkingBudget("claude-fable-5", "off"); got != 0 {
		t.Errorf("fable off = %d, want 0", got)
	}
	if got := resolveThinkingBudget("claude-fable-5", "high"); got != 16384 {
		t.Errorf("fable high = %d, want 16384", got)
	}
}
