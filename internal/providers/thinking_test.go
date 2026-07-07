package providers

import "testing"

func TestUsesAdaptiveThinking(t *testing.T) {
	adaptive := []string{
		"claude-fable-5", "Claude-Fable-5", "mythos-5-preview",
		"claude-opus-4-8", "claude-opus-4-7", "anthropic/claude-opus-4.8",
		"claude-sonnet-5",
	}
	for _, m := range adaptive {
		if !UsesAdaptiveThinking(m) {
			t.Errorf("UsesAdaptiveThinking(%q) = false, want true", m)
		}
	}
	legacy := []string{"claude-opus-4-6", "claude-sonnet-4-6", "claude-haiku-4-5", "MiniMax-M1", "gpt-4o", ""}
	for _, m := range legacy {
		if UsesAdaptiveThinking(m) {
			t.Errorf("UsesAdaptiveThinking(%q) = true, want false", m)
		}
	}
}

func TestAlwaysOnThinking(t *testing.T) {
	if !AlwaysOnThinking("claude-fable-5") || !AlwaysOnThinking("mythos-5") {
		t.Error("Fable/Mythos should be always-on")
	}
	if AlwaysOnThinking("claude-opus-4-8") {
		t.Error("Opus 4.8 is adaptive but not always-on")
	}
}

func TestEffortForThinkingBudget(t *testing.T) {
	cases := map[int]string{0: "", 1024: "low", 2048: "low", 8192: "medium", 16384: "high"}
	for budget, want := range cases {
		if got := EffortForThinkingBudget(budget); got != want {
			t.Errorf("EffortForThinkingBudget(%d) = %q, want %q", budget, got, want)
		}
	}
}

func TestThinkingFor(t *testing.T) {
	// Adaptive class: budget translates to adaptive + effort, no budget_tokens.
	p, cfg, mt := thinkingFor("claude-opus-4-8", 16384, 4096)
	if p == nil || p.Type != "adaptive" || p.BudgetTokens != 0 || p.Display != "summarized" {
		t.Errorf("adaptive high: got %+v", p)
	}
	if cfg == nil || cfg.Effort != "high" {
		t.Errorf("adaptive high effort: got %+v", cfg)
	}
	if mt != 4096 {
		t.Errorf("adaptive should not bump max_tokens: got %d", mt)
	}
	// Adaptive class, off: explicit disabled.
	p, cfg, _ = thinkingFor("claude-sonnet-5", 0, 4096)
	if p == nil || p.Type != "disabled" || cfg != nil {
		t.Errorf("adaptive off: got %+v cfg %+v", p, cfg)
	}
	// Always-on class, off: field omitted entirely (disabled 400s on Fable).
	p, cfg, _ = thinkingFor("claude-fable-5", 0, 4096)
	if p != nil || cfg != nil {
		t.Errorf("fable off should omit thinking: got %+v cfg %+v", p, cfg)
	}
	// Legacy class: enabled + budget, max_tokens bumped above the budget.
	p, cfg, mt = thinkingFor("claude-haiku-4-5", 8192, 4096)
	if p == nil || p.Type != "enabled" || p.BudgetTokens != 8192 || cfg != nil {
		t.Errorf("legacy: got %+v cfg %+v", p, cfg)
	}
	if mt <= 8192 {
		t.Errorf("legacy max_tokens must exceed budget: got %d", mt)
	}
	// Legacy class, off: omitted.
	p, cfg, _ = thinkingFor("MiniMax-M1", 0, 4096)
	if p != nil || cfg != nil {
		t.Errorf("legacy off should omit thinking: got %+v cfg %+v", p, cfg)
	}
}
