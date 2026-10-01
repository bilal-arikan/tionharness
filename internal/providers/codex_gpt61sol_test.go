package providers

import "testing"

func TestGPT61SolAliases(t *testing.T) {
	for _, model := range []string{
		"gpt-6.1-sol", " GPT-6.1-SOL ", "openai/gpt-6.1-sol", "gpt6.1-sol",
	} {
		t.Run(model, func(t *testing.T) {
			if got := gpt6Tier(model); got != "sol" {
				t.Fatalf("tier = %q, want sol", got)
			}
			if got := ContextWindowFor("codex-cli", model); got != codexGPT6ContextWindow {
				t.Fatalf("CLI context = %d, want %d", got, codexGPT6ContextWindow)
			}
			if err := ValidateThinkingLevelForProvider("codex-cli", model, "off"); err == nil {
				t.Fatal("unsupported off level accepted for alias")
			}
			if err := ValidateThinkingLevelForProvider("codex-cli", model, "ultra"); err != nil {
				t.Fatalf("supported ultra level rejected: %v", err)
			}
		})
	}
	for _, model := range []string{"gpt-6.1-luna", "gpt-6.1-astra", "gpt-6.2-sol"} {
		if got := gpt6Tier(model); got != "" {
			t.Errorf("unverified model %q classified as %q", model, got)
		}
	}
}
