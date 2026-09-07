package providers

import (
	"slices"
	"testing"
)

// TestGPT6AstraFamilyGates pins the family gate widening: the GPT-6 generation
// must clear gptFamily so it inherits the window / max-output / budget-fraction
// tables, while the pre-5 slugs the gate deliberately excludes stay excluded.
func TestGPT6AstraFamilyGates(t *testing.T) {
	// gptFamily is an internal helper: callers lower-case the slug first, so the
	// gate itself is only ever fed lower-case input. Mixed case is covered at the
	// exported layer in TestGPT6AstraWindowAndOutput.
	for _, m := range []string{"gpt-6-astra", "gpt6-astra", "openai/gpt-6-astra"} {
		if !gptFamily(m) {
			t.Errorf("gptFamily(%q) = false, want true", m)
		}
	}
	for _, m := range []string{"gpt-4o-mini", "gpt-4.1", "claude-opus-5", ""} {
		if gptFamily(m) {
			t.Errorf("gptFamily(%q) = true, want false", m)
		}
	}
}

// TestGPT6AstraWindowAndOutput pins Astra to the large GPT tier rather than the
// 272K CLI fallback. 272K is Astra's long-context *pricing* threshold, not its
// window, so landing on windowGPTOther would under-report the window by ~4x and
// compact far too early.
func TestGPT6AstraWindowAndOutput(t *testing.T) {
	for _, m := range []string{"gpt-6-astra", "GPT-6-Astra", " gpt-6-astra "} {
		if got := ContextWindowFor("codex-cli", m); got != windowGPTLarge {
			t.Errorf("window(%q) = %d, want %d", m, got, windowGPTLarge)
		}
	}
	if got := MaxOutputFor("codex-cli", "gpt-6-astra"); got != maxOutGPT {
		t.Errorf("max output = %d, want %d", got, maxOutGPT)
	}
	if got := AdaptiveBudgetFraction("codex-cli", "gpt-6-astra"); got != 0.35 {
		t.Errorf("budget fraction = %v, want 0.35", got)
	}
}

// TestGPT6AstraThinkingRamp pins the effort ramp: Astra documents low/medium/
// high/xhigh plus max, so on the Codex transport it gets the same full ramp as
// the GPT-5.6 tiers — including "ultra", which is a CLI-only effort value.
func TestGPT6AstraThinkingRamp(t *testing.T) {
	want := []string{"off", "low", "medium", "high", "xhigh", "max", "ultra"}
	if got := ThinkingTiersForProvider("codex-cli", "gpt-6-astra"); !slices.Equal(got, want) {
		t.Fatalf("tiers = %v, want %v", got, want)
	}
	if got := ThinkingClassForProvider("codex-cli", "gpt-6-astra"); got != "adaptive" {
		t.Fatalf("class = %q, want adaptive", got)
	}
	for _, level := range want {
		if err := ValidateThinkingLevelForProvider("codex-cli", "gpt-6-astra", level); err != nil {
			t.Errorf("level %q rejected: %v", level, err)
		}
	}
	// The widening is Codex-specific: another transport must not inherit "ultra".
	if got := ThinkingTiersForProvider("claude-cli", "gpt-6-astra"); slices.Contains(got, "ultra") {
		t.Errorf("non-Codex transport got the CLI-only tier: %v", got)
	}
}

// TestGPT6AstraPrice pins the launch pricing: $10/$50 with a $1/MTok cached read
// (0.10x, same ratio as its siblings) but a 1.25x cache WRITE, which the other
// OpenAI entries pin to 1.0 because they carry no write premium.
func TestGPT6AstraPrice(t *testing.T) {
	p, ok := PriceFor("openai", "gpt-6-astra")
	if !ok {
		t.Fatal("gpt-6-astra price missing")
	}
	if p.InputPerMTok != 10 || p.OutputPerMTok != 50 {
		t.Errorf("price = %v/%v, want 10/50", p.InputPerMTok, p.OutputPerMTok)
	}
	if p.CacheReadMultOverride != 0.10 {
		t.Errorf("cache read mult = %v, want 0.10 ($1/MTok)", p.CacheReadMultOverride)
	}
	if p.CacheWriteMultOverride != 1.25 {
		t.Errorf("cache write mult = %v, want 1.25 ($12.50/MTok)", p.CacheWriteMultOverride)
	}
}

// TestGPT6AstraInCatalog pins the catalog entry and its resolved metadata, the
// same way kind_test.go pins gpt-5.6-sol.
func TestGPT6AstraInCatalog(t *testing.T) {
	for _, entry := range Catalog() {
		if entry.ID != "codex-cli" {
			continue
		}
		for _, model := range entry.Models {
			if model.ID != "gpt-6-astra" {
				continue
			}
			if model.ThinkingClass != "adaptive" || !slices.Contains(model.ThinkingTiers, "ultra") {
				t.Fatalf("metadata = tiers %v class %q", model.ThinkingTiers, model.ThinkingClass)
			}
			return
		}
	}
	t.Fatal("codex-cli/gpt-6-astra missing from catalog")
}
