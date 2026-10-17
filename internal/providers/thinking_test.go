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
		return slices.Contains(tiers, v)
	}
	// Always-on class: no "off" (thinking cannot be disabled), full depth ramp.
	fable := ThinkingTiersForProvider("", "claude-fable-5")
	if has(fable, "off") {
		t.Errorf("fable should not offer off: %v", fable)
	}
	if !has(fable, "max") || !has(fable, "xhigh") {
		t.Errorf("fable should offer xhigh/max: %v", fable)
	}
	// Adaptive class: full ramp including off + xhigh/max.
	adaptive := ThinkingTiersForProvider("", "claude-opus-4-8")
	for _, v := range []string{"off", "low", "medium", "high", "xhigh", "max"} {
		if !has(adaptive, v) {
			t.Errorf("adaptive should offer %q: %v", v, adaptive)
		}
	}
	// Concrete legacy model: off/low/medium/high, but NOT xhigh/max (they clamp).
	legacy := ThinkingTiersForProvider("", "claude-haiku-4-5-20251001")
	if has(legacy, "xhigh") || has(legacy, "max") {
		t.Errorf("legacy should not offer xhigh/max: %v", legacy)
	}
	if !has(legacy, "off") || !has(legacy, "high") {
		t.Errorf("legacy should offer off..high: %v", legacy)
	}
	// Bare alias / custom / empty: full ramp (provider clamps).
	fullRamp := []string{"off", "low", "medium", "high", "xhigh", "max", "ultra"}
	for _, m := range []string{"opus", "sonnet", ""} {
		full := ThinkingTiersForProvider("", m)
		if !slices.Equal(full, fullRamp) {
			t.Errorf("alias %q should get full ramp %v: %v", m, fullRamp, full)
		}
	}
	// Coarse effort (DeepSeek V4.x): off plus the three real levels.
	effort := []string{"off", "low", "high", "max"}
	for _, m := range []string{"deepseek-flash", "deepseek-v4-flash", "deepseek-v4-pro"} {
		if got := ThinkingTiersForProvider("", m); !slices.Equal(got, effort) {
			t.Errorf("%s should offer %v: %v", m, effort, got)
		}
	}
	// Forced thinking (GLM-5.3 family): the same levels without "off".
	for _, m := range []string{"glm-5.3", "glm-5.3-flash", "glm-5.3-flashx"} {
		if got := ThinkingTiersForProvider("", m); !slices.Equal(got, []string{"low", "high", "max"}) {
			t.Errorf("%s should offer low/high/max only: %v", m, got)
		}
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

// TestCLITransportsOfferUltra pins that the CLI kinds keep the "ultra" tier
// (reached through CLAUDE_CODE_EFFORT_LEVEL / codex model_reasoning_effort).
func TestCLITransportsOfferUltra(t *testing.T) {
	if got := ThinkingTiersForProvider("claude-cli", "claude-opus-4-8"); !slices.Contains(got, "ultra") {
		t.Errorf("claude-cli lost ultra, which it reaches through CLAUDE_CODE_EFFORT_LEVEL: %v", got)
	}
	if got := StorableThinkingLevelsFor("claude-cli", "claude-haiku-4-5"); slices.Contains(got, "ultra") {
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
		"claude-fable-5":               "always-on",
		"mythos-5":                     "always-on",
		"claude-opus-4-8":              "adaptive",
		"claude-sonnet-5":              "adaptive",
		"deepseek-flash":               "effort", // no digit, still not an alias
		"deepseek-v4-flash":            "effort", // routed to V4.1 Flash, which reasons
		"deepseek/deepseek-v4-flash":   "effort",
		"deepseek-v4-pro":              "effort",
		"glm-5.3":                      "effort",
		"glm-5.3-flash":                "effort",
		"z-ai/glm-5.3":                 "effort",
		"glm-5.2":                      "legacy", // can disable reasoning; effort not verified
		"deepseek-r1-distill-qwen-32b": "legacy", // local distill, not the hosted V4 wire
		"claude-haiku-4-5":             "legacy",
		"MiniMax-M3":                   "legacy",
		"opus":                         "alias",
		"":                             "alias",
	}
	for model, want := range cases {
		if got := ThinkingClass(model); got != want {
			t.Errorf("ThinkingClass(%q) = %q, want %q", model, got, want)
		}
	}
}
