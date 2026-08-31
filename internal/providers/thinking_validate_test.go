package providers

import (
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
		{"non-thinking model takes off only", "deepseek-v4-flash", "off", false},
		{"non-thinking model rejects high", "deepseek-v4-flash", "high", true},
		{"always-on model rejects off", "claude-fable-5", "off", true},
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

// TestValidateThinkingLevelErrorNamesTheAlternatives: the message has to tell the
// caller what it could have sent instead, otherwise a 400 is a dead end.
func TestValidateThinkingLevelErrorNamesTheAlternatives(t *testing.T) {
	err := ValidateThinkingLevel("deepseek-v4-flash", "high")
	if err == nil {
		t.Fatal("expected an error")
	}
	if got := err.Error(); !strings.Contains(got, "supported: off") {
		t.Fatalf("error should list the supported tiers, got: %s", got)
	}
}
