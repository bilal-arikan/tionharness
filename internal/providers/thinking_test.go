package providers

import (
	"slices"
	"testing"
)

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

func TestThinkingTiersFor(t *testing.T) {
	has := func(tiers []string, v string) bool {
		for _, t := range tiers {
			if t == v {
				return true
			}
		}
		return false
	}
	// Always-on class: no "off" (thinking cannot be disabled), full depth ramp.
	fable := ThinkingTiersFor("claude-fable-5")
	if has(fable, "off") {
		t.Errorf("fable should not offer off: %v", fable)
	}
	if !has(fable, "max") || !has(fable, "xhigh") {
		t.Errorf("fable should offer xhigh/max: %v", fable)
	}
	// Adaptive class: full ramp including off + xhigh/max.
	adaptive := ThinkingTiersFor("claude-opus-4-8")
	for _, v := range []string{"off", "low", "medium", "high", "xhigh", "max"} {
		if !has(adaptive, v) {
			t.Errorf("adaptive should offer %q: %v", v, adaptive)
		}
	}
	// Concrete legacy model: off/low/medium/high, but NOT xhigh/max (they clamp).
	legacy := ThinkingTiersFor("claude-haiku-4-5-20251001")
	if has(legacy, "xhigh") || has(legacy, "max") {
		t.Errorf("legacy should not offer xhigh/max: %v", legacy)
	}
	if !has(legacy, "off") || !has(legacy, "high") {
		t.Errorf("legacy should offer off..high: %v", legacy)
	}
	// Bare alias / custom / empty: full ramp (provider clamps).
	fullRamp := []string{"off", "low", "medium", "high", "xhigh", "max", "ultra"}
	for _, m := range []string{"opus", "sonnet", ""} {
		full := ThinkingTiersFor(m)
		if !slices.Equal(full, fullRamp) {
			t.Errorf("alias %q should get full ramp %v: %v", m, fullRamp, full)
		}
	}
	// Non-thinking (DeepSeek Flash): only "off".
	flash := ThinkingTiersFor("deepseek-v4-flash")
	if len(flash) != 1 || flash[0] != "off" {
		t.Errorf("deepseek flash should offer only off: %v", flash)
	}
}

func TestCodexGPT5ThinkingTiersAreProviderAware(t *testing.T) {
	want := []string{"off", "low", "medium", "high", "xhigh", "max", "ultra"}
	if got := ThinkingTiersForProvider("codex-cli", "gpt-5.6-sol"); !slices.Equal(got, want) {
		t.Fatalf("codex gpt-5.6-sol tiers = %v, want %v", got, want)
	}
	if got := ThinkingClassForProvider("codex-cli", "gpt-5.6-sol"); got != "adaptive" {
		t.Fatalf("codex gpt-5.6-sol class = %q, want adaptive", got)
	}
	if got := ThinkingTiersForProvider("claude-cli", "gpt-5.6-sol"); slices.Contains(got, "ultra") {
		t.Fatalf("non-Codex concrete model unexpectedly got full ramp: %v", got)
	}
}

// TestNativeEffortTransportsDoNotOfferUltra pins the asymmetry the effort enum
// forces: "ultra" is a real CLI effort value but has no Messages-API
// representation, so the HTTP kinds must not offer it while the CLI kinds must.
func TestNativeEffortTransportsDoNotOfferUltra(t *testing.T) {
	for _, kind := range []string{"anthropic", "anthropic-compat"} {
		got := ThinkingTiersForProvider(kind, "claude-opus-4-8")
		if slices.Contains(got, "ultra") {
			t.Errorf("%s offers ultra but output_config.effort cannot carry it: %v", kind, got)
		}
		if !slices.Contains(got, "max") {
			t.Errorf("%s lost the max tier: %v", kind, got)
		}
		// Always-on models take the same treatment.
		if alwaysOn := ThinkingTiersForProvider(kind, "claude-fable-5"); slices.Contains(alwaysOn, "ultra") {
			t.Errorf("%s always-on ramp offers ultra: %v", kind, alwaysOn)
		}
	}
	if got := ThinkingTiersForProvider("claude-cli", "claude-opus-4-8"); !slices.Contains(got, "ultra") {
		t.Errorf("claude-cli lost ultra, which it reaches through CLAUDE_CODE_EFFORT_LEVEL: %v", got)
	}
}

// TestUltraStaysStorableOnNativeEffortTransports guards the migration edge: an
// agent that stored "ultra" (picked on a CLI provider, or before a provider
// switch) must remain saveable even though the picker no longer offers it.
func TestUltraStaysStorableOnNativeEffortTransports(t *testing.T) {
	for _, kind := range []string{"anthropic", "anthropic-compat"} {
		if got := StorableThinkingLevelsFor(kind, "claude-opus-4-8"); !slices.Contains(got, "ultra") {
			t.Errorf("%s made a stored ultra row unsaveable: %v", kind, got)
		}
		if err := ValidateThinkingLevelForProvider(kind, "claude-opus-4-8", "ultra"); err != nil {
			t.Errorf("%s rejected a stored ultra level: %v", kind, err)
		}
	}
	// A model class that never had ultra does not gain it.
	if got := StorableThinkingLevelsFor("anthropic", "claude-haiku-4-5"); slices.Contains(got, "ultra") {
		t.Errorf("legacy class gained ultra: %v", got)
	}
}

// TestEffortForThinkingBudgetUltraReportsTheEnumCeiling documents that the
// ultra budget maps to "max" deliberately — the Messages API effort enum is
// low|medium|high|xhigh|max — rather than by an unmarked default fallthrough.
func TestEffortForThinkingBudgetUltraReportsTheEnumCeiling(t *testing.T) {
	if got := EffortForThinkingBudget(65536); got != "max" {
		t.Errorf("max budget = %q, want max", got)
	}
	if got := EffortForThinkingBudget(131072); got != "max" {
		t.Errorf("ultra budget = %q, want max (effort enum has no ultra)", got)
	}
}

func TestThinkingClass(t *testing.T) {
	cases := map[string]string{
		"claude-fable-5":             "always-on",
		"mythos-5":                   "always-on",
		"claude-opus-4-8":            "adaptive",
		"claude-sonnet-5":            "adaptive",
		"deepseek-v4-flash":          "non-thinking",
		"deepseek/deepseek-v4-flash": "non-thinking",
		"deepseek-v4-pro":            "legacy", // Pro reasons — not lumped with Flash
		"claude-haiku-4-5":           "legacy",
		"MiniMax-M3":                 "legacy",
		"opus":                       "alias",
		"":                           "alias",
	}
	for model, want := range cases {
		if got := ThinkingClass(model); got != want {
			t.Errorf("ThinkingClass(%q) = %q, want %q", model, got, want)
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
