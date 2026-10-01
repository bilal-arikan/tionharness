package providers

import (
	"slices"
	"testing"
)

func TestClaude55Capabilities(t *testing.T) {
	for _, model := range []string{"claude-opus-5-5", "claude-sonnet-5-5", "anthropic/claude-opus-5.5", "anthropic/claude-sonnet-5.5"} {
		t.Run(model, func(t *testing.T) {
			if !UsesAdaptiveThinking(model) || !SupportsThinkingBinding(model) || !SupportsSystemInMessages(model) || !SupportsTaskBudget(model) || !SupportsStructuredOutputs(model) || !SupportsDynamicWebTools(model) || !SupportsProgrammaticTools(model) {
				t.Fatal("missing Claude 5.5 capability")
			}
			if got := ContextWindowFor("anthropic", model); got != 1_000_000 {
				t.Fatalf("context = %d, want 1000000", got)
			}
			if tiers := ThinkingTiersForProvider("anthropic", model); !slices.Contains(tiers, "max") || slices.Contains(tiers, "ultra") {
				t.Fatalf("native effort tiers = %v", tiers)
			}
		})
	}
	if !AlwaysOnThinking("claude-opus-5-5") || AlwaysOnThinking("claude-opus-5") || AlwaysOnThinking("claude-sonnet-5-5") {
		t.Fatal("Opus 5.5 must be the only newly always-on model")
	}
	if SupportsThinkingBinding("claude-sonnet-5") {
		t.Fatal("Sonnet 5 must retain its earlier behavior")
	}
}

func TestClaude55Catalog(t *testing.T) {
	for _, provider := range []string{"anthropic", "claude-cli", "openrouter"} {
		for _, model := range []string{"claude-opus-5-5", "claude-sonnet-5-5"} {
			id := model
			if provider == "openrouter" {
				if model == "claude-opus-5-5" {
					id = "anthropic/claude-opus-5.5"
				} else {
					id = "anthropic/claude-sonnet-5.5"
				}
			}
			found := false
			for _, entry := range Catalog() {
				if entry.ID != provider {
					continue
				}
				for _, info := range entry.Models {
					if info.ID == id {
						found = true
						if info.ContextWindow != 1_000_000 {
							t.Errorf("%s/%s context = %d", provider, id, info.ContextWindow)
						}
					}
				}
			}
			if !found {
				t.Errorf("%s/%s missing from catalog", provider, id)
			}
		}
	}
}

func TestClaude55Pricing(t *testing.T) {
	for _, tc := range []struct {
		model, router       string
		input, output, read float64
	}{
		{"claude-opus-5-5", "anthropic/claude-opus-5.5", 4, 20, 0.2},
		{"claude-sonnet-5-5", "anthropic/claude-sonnet-5.5", 2, 10, 0.2},
	} {
		for _, provider := range []string{"anthropic", "openrouter", "claude-cli"} {
			model := tc.model
			if provider == "openrouter" {
				model = tc.router
			}
			p, ok := PriceFor(provider, model)
			if provider == "claude-cli" {
				if ok {
					t.Fatal("subscription model must not become API billing")
				}
				p, ok = EstimateFor(provider, model)
			}
			if !ok || p.InputPerMTok != tc.input || p.OutputPerMTok != tc.output {
				t.Errorf("%s/%s price = %+v, found %v", provider, model, p, ok)
			}
			if got := p.CostDetailed(0, 0, 1_000_000, 0); !approx(got, tc.read) {
				t.Errorf("%s/%s cache read = %v, want %v", provider, model, got, tc.read)
			}
			write := tc.input * 1.25
			if provider == "anthropic" {
				write = tc.input * 2
			}
			if got := p.CostDetailed(0, 0, 0, 1_000_000); !approx(got, write) {
				t.Errorf("%s/%s cache write = %v, want %v", provider, model, got, write)
			}
		}
	}
}
