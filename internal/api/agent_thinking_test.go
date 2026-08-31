package api

import (
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// TestExplicitThinkingLevel pins the resolution the definition-driven creation
// paths (pack install, template seeding, template publish) share: a level named
// by the definition survives verbatim — including "off", which must not be
// mistaken for "unset" — and a missing one falls back to the legacy rule for the
// provider kind, never to the empty string.
func TestExplicitThinkingLevel(t *testing.T) {
	cases := []struct {
		name     string
		level    string
		provider string
		want     string
	}{
		{"explicit level wins", "medium", "anthropic", "medium"},
		{"explicit off is not 'unset'", "off", "claude-cli", "off"},
		{"levelless CLI pack", "", "claude-cli", "high"},
		{"levelless codex pack", "", "codex-cli", "high"},
		{"levelless native pack", "", "anthropic", "off"},
		{"levelless, provider unknown", "", "", "high"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := explicitThinkingLevel(tc.level, tc.provider)
			if got != tc.want {
				t.Fatalf("explicitThinkingLevel(%q, %q) = %q, want %q", tc.level, tc.provider, got, tc.want)
			}
			if got == "" {
				t.Fatal("resolution must never yield the empty (invalid) level")
			}
		})
	}
}

// TestExplicitThinkingLevelMatchesStoreFallback guards against the call-site
// resolution drifting from db.CreateAgent's remaining safety net: both must pick
// the same tier for a levelless definition, or a pack would install differently
// depending on which layer resolved it.
func TestExplicitThinkingLevelMatchesStoreFallback(t *testing.T) {
	for _, kind := range []string{"", "claude-cli", "codex-cli", "anthropic", "openai", "minimax"} {
		if got, want := explicitThinkingLevel("", kind), db.LegacyThinkingLevelFor(kind); got != want {
			t.Fatalf("provider %q: call site resolved %q but the store would use %q", kind, got, want)
		}
	}
}
