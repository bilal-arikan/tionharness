package providers

import "testing"

func TestContextWindowFor(t *testing.T) {
	cases := []struct {
		model string
		want  int
	}{
		{"claude-opus-4-8", windowClaudeOpusSonnet},
		{"claude-haiku-4-5-20251001", windowHaiku},
		{"anthropic/claude-sonnet-4.6", windowClaudeOpusSonnet},
		{"anthropic/claude-opus-4.8-fast", windowClaudeOpusSonnet},
		{"opus", windowClaudeOpusSonnet},
		{"sonnet", windowClaudeOpusSonnet},
		{"haiku", windowHaiku},
		{"claude-fable-5", windowFable},
		{"fable", windowFable},
		{"MiniMax-M3", windowMiniMax},
		{"minimax/minimax-m3", windowMiniMax},
		{"deepseek/deepseek-v4-flash", windowDeepSeek},
		{"google/gemini-3.5-flash", windowGemini},
		{"gpt-6-astra", windowGPTLarge},
		{"gpt-5.6-sol", windowGPTLarge},
		{"gpt-5.6-terra", windowGPTLarge},
		{"gpt-5.6-luna", windowGPTLuna},
		{"gpt-5.5", windowGPTOther},
		{"codex-auto-review", windowGPTOther},
		{"openai/gpt-5.5", windowGPTOther}, // openrouter id, same family gate
		{"gpt-5.4-mini", windowGPTOther},
		// Pre-5 GPT slugs are NOT claimed: 272K would over-estimate them (4o-mini is
		// really 128K) and compaction would fire too late.
		{"gpt-4o-mini", 0},
		{"openai/gpt-4o", 0},
		{"gpt-4.1", 0},
		{"glm-5.3", windowGLMLarge},
		{"glm-5.2", windowGLMLarge},
		{"z-ai/glm-5.2", windowGLMLarge},
		{"glm-5.1", windowGLMOther},
		{"glm-5", windowGLMOther},
		{"glm-5-turbo", windowGLMOther},
		{"glm-4.7", windowGLMOther},
		{"glm-4.7-flash", windowGLMOther},
		{"glm-4.6", windowGLMOther},
		{"", 0},                                     // claude-cli default → unknown
		{"some-unknown-model", 0},                   // unknown → 0 (caller falls back)
		{"  Claude-Opus  ", windowClaudeOpusSonnet}, // trimmed + case-insensitive
	}
	for _, c := range cases {
		if got := ContextWindowFor("", c.model); got != c.want {
			t.Errorf("ContextWindowFor(%q) = %d, want %d", c.model, got, c.want)
		}
	}
}

func TestAdaptiveBudgetFraction(t *testing.T) {
	cases := []struct {
		model string
		want  float64
	}{
		{"claude-opus-4-8", 0.45},
		{"anthropic/claude-sonnet-4.6", 0.45},
		{"claude-haiku-4-5-20251001", 0.40},
		{"claude-fable-5", 0.45},
		{"MiniMax-M3", 0.35},
		{"deepseek/deepseek-v4-flash", 0.35},
		{"google/gemini-3.5-flash", 0.35},
		{"", 0},                  // claude-cli default → unknown → 0 (caller falls back)
		{"openai/gpt-5.5", 0.35}, // gpt/codex family → long-context share
		{"gpt-6-astra", 0.35},
		{"gpt-5.6-sol", 0.35},
		{"gpt-4o-mini", 0}, // no longer in the gpt gate → unknown
		{"glm-5.2", 0.35},
		{"glm-4.7", 0.35},
	}
	for _, c := range cases {
		if got := AdaptiveBudgetFraction("", c.model); got != c.want {
			t.Errorf("AdaptiveBudgetFraction(%q) = %v, want %v", c.model, got, c.want)
		}
	}
}

func TestCatalogFillsContextWindow(t *testing.T) {
	for _, entry := range Catalog() {
		for _, m := range entry.Models {
			want := ContextWindowFor(entry.ID, m.ID)
			if m.ContextWindow != want {
				t.Errorf("catalog %s/%s ContextWindow = %d, want %d", entry.ID, m.ID, m.ContextWindow, want)
			}
		}
	}
}
