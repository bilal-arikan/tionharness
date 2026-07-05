package agent

import (
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/providers"
)

func TestWithMaxOutput(t *testing.T) {
	r := &Runtime{tun: NewTunables()}

	// Unset cap is filled from the model family.
	got := r.withMaxOutput("anthropic", providers.Request{Model: "claude-opus-4-8"})
	if got.MaxTokens != providers.MaxOutputFor("anthropic", "claude-opus-4-8") {
		t.Errorf("unset cap = %d, want family default", got.MaxTokens)
	}

	// An explicit cap (e.g. compaction) is never overridden.
	got = r.withMaxOutput("anthropic", providers.Request{Model: "claude-opus-4-8", MaxTokens: 8192})
	if got.MaxTokens != 8192 {
		t.Errorf("explicit cap overridden: got %d, want 8192", got.MaxTokens)
	}

	// Unknown family leaves the cap unset so the provider falls back to its default.
	got = r.withMaxOutput("openrouter", providers.Request{Model: "openai/gpt-5.5"})
	if got.MaxTokens != 0 {
		t.Errorf("unknown family = %d, want 0 (provider fallback)", got.MaxTokens)
	}
}

func TestWithMaxOutputSettingsOverride(t *testing.T) {
	r := &Runtime{tun: NewTunables()}
	r.tun.SetMaxOutputTokens(20000)

	// The settings override wins over the per-family default when the cap is unset.
	got := r.withMaxOutput("anthropic", providers.Request{Model: "claude-opus-4-8"})
	if got.MaxTokens != 20000 {
		t.Errorf("settings override unset cap = %d, want 20000", got.MaxTokens)
	}

	// It even applies to an otherwise-unknown family (it's a global cap).
	got = r.withMaxOutput("openrouter", providers.Request{Model: "openai/gpt-5.5"})
	if got.MaxTokens != 20000 {
		t.Errorf("settings override unknown family = %d, want 20000", got.MaxTokens)
	}

	// But an explicit cap still wins over the settings override.
	got = r.withMaxOutput("anthropic", providers.Request{Model: "claude-opus-4-8", MaxTokens: 8192})
	if got.MaxTokens != 8192 {
		t.Errorf("settings override beat explicit cap: got %d, want 8192", got.MaxTokens)
	}
}

func TestWithMaxOutputEnvOverride(t *testing.T) {
	defer func(prev int) { maxOutputOverride = prev }(maxOutputOverride)
	maxOutputOverride = 12345
	r := &Runtime{tun: NewTunables()} // no settings override → env applies

	got := r.withMaxOutput("anthropic", providers.Request{Model: "claude-opus-4-8"})
	if got.MaxTokens != 12345 {
		t.Errorf("env override unset cap = %d, want 12345", got.MaxTokens)
	}

	// Settings override takes precedence over the env when both are set.
	r.tun.SetMaxOutputTokens(20000)
	got = r.withMaxOutput("anthropic", providers.Request{Model: "claude-opus-4-8"})
	if got.MaxTokens != 20000 {
		t.Errorf("settings did not beat env: got %d, want 20000", got.MaxTokens)
	}
}
