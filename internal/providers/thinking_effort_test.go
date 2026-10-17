package providers

import (
	"testing"
)

// TestUsesCoarseEffort pins the family gate: DeepSeek's hosted V4 generation and
// the GLM-5.3 line, on any transport — but not local distills, the retired
// DeepSeek aliases, or GLM-5.2 and older.
func TestUsesCoarseEffort(t *testing.T) {
	yes := []string{
		"deepseek-flash", "deepseek-v4-flash", "deepseek-v4-pro", "deepseek-v4-flash-vision-exp",
		"deepseek/deepseek-v4.1-flash", "DeepSeek-Flash",
		"glm-5.3", "glm-5.3-flash", "glm-5.3-flashx", "z-ai/glm-5.3",
	}
	for _, m := range yes {
		if !UsesCoarseEffort(m) {
			t.Errorf("UsesCoarseEffort(%q) = false, want true", m)
		}
	}
	no := []string{
		"deepseek-r1-distill-qwen-32b", "deepseek-chat", "deepseek-reasoner",
		"glm-5.2", "glm-5.1", "glm-5", "glm-4.7-flash", "claude-opus-4-8", "MiniMax-M3", "",
	}
	for _, m := range no {
		if UsesCoarseEffort(m) {
			t.Errorf("UsesCoarseEffort(%q) = true, want false", m)
		}
	}
}

// TestForcedThinking: only the GLM-5.3 family cannot stop reasoning; DeepSeek
// V4.x can be switched off.
func TestForcedThinking(t *testing.T) {
	for _, m := range []string{"glm-5.3", "glm-5.3-flash", "glm-5.3-flashx", "z-ai/glm-5.3"} {
		if !ForcedThinking(m) {
			t.Errorf("ForcedThinking(%q) = false, want true", m)
		}
	}
	for _, m := range []string{"deepseek-flash", "deepseek-v4-pro", "glm-5.2", "claude-fable-5"} {
		if ForcedThinking(m) {
			t.Errorf("ForcedThinking(%q) = true, want false", m)
		}
	}
}

// TestCoarseEffortForBudget pins the fold of every agent tier onto low/high/max,
// using the budgets thinkingBudgetForLevel produces.
func TestCoarseEffortForBudget(t *testing.T) {
	cases := []struct {
		budget int
		forced bool
		want   string
	}{
		{0, false, ""},       // off → omitted, the caller disables reasoning
		{0, true, "low"},     // off on a forced model → the lowest level
		{2048, false, "low"}, // low
		{8192, false, "high"},
		{16384, false, "high"},
		{32768, false, "max"}, // xhigh
		{65536, false, "max"},
		{131072, true, "max"}, // ultra
	}
	for _, c := range cases {
		if got := CoarseEffortForBudget(c.budget, c.forced); got != c.want {
			t.Errorf("CoarseEffortForBudget(%d, %v) = %q, want %q", c.budget, c.forced, got, c.want)
		}
	}
}
