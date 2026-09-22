package providers

import (
	"slices"
	"strings"
	"testing"
)

func TestIsValidThinkingLevel(t *testing.T) {
	for _, level := range ValidThinkingLevels() {
		if !IsValidThinkingLevel(level) {
			t.Fatalf("%q should be valid", level)
		}
	}
	for _, level := range []string{"", "kapalı", "HIGH", "none"} {
		if IsValidThinkingLevel(level) {
			t.Fatalf("%q should not be valid", level)
		}
	}
}

// TestValidateThinkingLevel pins the two gates: the token must be known, and it
// must be a tier the model actually reasons at.
func TestValidateThinkingLevel(t *testing.T) {
	cases := []struct {
		name    string
		model   string
		level   string
		wantErr bool
	}{
		{"blank is rejected", "claude-opus-4-8", "", true},
		{"unknown token is rejected", "claude-opus-4-8", "extreme", true},
		{"adaptive model takes the whole ramp", "claude-opus-4-8", "max", false},
		{"legacy model clamps xhigh away", "claude-opus-4-6", "xhigh", true},
		{"legacy model still takes high", "claude-opus-4-6", "high", false},
		{"effort model takes max", "deepseek-flash", "max", false},
		{"effort model takes off", "deepseek-flash", "off", false},
		// medium was offered while DeepSeek sat in the legacy class; it folds
		// onto high, so rows saved back then stay saveable.
		{"effort model stores medium", "deepseek-v4-pro", "medium", false},
		{"effort model rejects ultra", "deepseek-flash", "ultra", true},
		// GLM-5.3 cannot stop reasoning; "off" is stored and sent as the lowest
		// effort, the always-on pattern.
		{"forced-thinking model stores off", "glm-5.3", "off", false},
		{"forced-thinking model rejects ultra", "glm-5.3-flash", "ultra", true},
		// "off" on the always-on class means "omit the thinking field", which is
		// what that wire format wants anyway, so it is a legal stored value even
		// though ThinkingTiersFor keeps it out of the offered picker set.
		{"always-on model stores off", "claude-fable-5", "off", false},
		{"always-on model still rejects an unknown token", "claude-fable-5", "extreme", true},
		{"always-on model takes a tier", "claude-fable-5", "medium", false},
		{"bare alias accepts anything valid", "opus", "max", false},
		{"empty model id is an alias too", "", "xhigh", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateThinkingLevel(c.model, c.level)
			if c.wantErr && err == nil {
				t.Fatalf("model %q level %q: expected an error", c.model, c.level)
			}
			if !c.wantErr && err != nil {
				t.Fatalf("model %q level %q: unexpected error: %v", c.model, c.level, err)
			}
		})
	}
}

func TestValidateThinkingLevelForCodexGPT5(t *testing.T) {
	for _, level := range []string{"xhigh", "max", "ultra"} {
		if err := ValidateThinkingLevelForProvider("codex-cli", "gpt-5.6-sol", level); err != nil {
			t.Fatalf("codex gpt-5.6-sol %s: %v", level, err)
		}
	}
	if err := ValidateThinkingLevelForProvider("claude-cli", "gpt-5.6-sol", "ultra"); err == nil {
		t.Fatal("non-Codex concrete model must retain legacy validation")
	}
	if err := ValidateThinkingLevelForProvider("codex-cli", "gpt-5.6-sol", "turbo"); err == nil {
		t.Fatal("unknown Codex tier must be rejected")
	}
}

// TestValidateThinkingLevelErrorNamesTheAlternatives: the message has to tell the
// caller what it could have sent instead, otherwise a 400 is a dead end.
func TestValidateThinkingLevelErrorNamesTheAlternatives(t *testing.T) {
	err := ValidateThinkingLevel("claude-opus-4-6", "xhigh")
	if err == nil {
		t.Fatal("expected an error")
	}
	if got := err.Error(); !strings.Contains(got, "supported: off") {
		t.Fatalf("error should list the supported tiers, got: %s", got)
	}
}

// TestStorableThinkingLevelsKeepsPickerSetIntact: widening what may be STORED
// must not widen what the picker offers — the always-on class still must not
// advertise "off" as a way to stop the model reasoning.
func TestStorableThinkingLevelsKeepsPickerSetIntact(t *testing.T) {
	for _, tier := range ThinkingTiersFor("claude-fable-5") {
		if tier == "off" {
			t.Fatal(`ThinkingTiersFor("claude-fable-5") must not offer "off"`)
		}
	}
	if got := StorableThinkingLevels("claude-fable-5"); got[0] != "off" {
		t.Fatalf(`StorableThinkingLevels("claude-fable-5") should start with "off", got %v`, got)
	}
	// The effort class stores more than it offers (medium/xhigh fold onto a
	// real level), but must still keep "off" out of the forced family's picker.
	for _, tier := range ThinkingTiersFor("glm-5.3") {
		if tier == "off" {
			t.Fatal(`ThinkingTiersFor("glm-5.3") must not offer "off"`)
		}
	}
	for _, model := range []string{"deepseek-flash", "glm-5.3"} {
		storable := StorableThinkingLevels(model)
		for _, tier := range ThinkingTiersFor(model) {
			if !slices.Contains(storable, tier) {
				t.Fatalf("model %q: offered tier %q is not storable (%v)", model, tier, storable)
			}
		}
	}
	// Every other class stores exactly what it offers.
	for _, model := range []string{"claude-opus-4-8", "claude-opus-4-6", "glm-5.2", "opus", ""} {
		offered, storable := ThinkingTiersFor(model), StorableThinkingLevels(model)
		if strings.Join(offered, ",") != strings.Join(storable, ",") {
			t.Fatalf("model %q: offered %v != storable %v", model, offered, storable)
		}
	}
}
