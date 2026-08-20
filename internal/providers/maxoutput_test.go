package providers

import "testing"

func TestMaxOutputFor(t *testing.T) {
	cases := []struct {
		model string
		want  int
	}{
		{"claude-opus-4-8", maxOutClaudeCapable},
		{"anthropic/claude-sonnet-4.6", maxOutClaudeCapable},
		{"opus", maxOutClaudeCapable},
		{"sonnet", maxOutClaudeCapable},
		{"claude-haiku-4-5-20251001", maxOutClaudeSmall},
		{"haiku", maxOutClaudeSmall},
		{"claude-fable-5", maxOutClaudeCapable},
		{"fable", maxOutClaudeCapable},
		{"MiniMax-M3", maxOutMiniMax},
		{"minimax/minimax-m3", maxOutMiniMax},
		{"deepseek/deepseek-v4-flash", maxOutDeepSeek},
		{"google/gemini-3.5-flash", maxOutGemini},
		{"", 0},                       // claude-cli default → unknown
		{"openai/gpt-5.5", maxOutGPT}, // gpt/codex family
		{"gpt-5.6-sol", maxOutGPT},
		{"gpt-5.6-luna", maxOutGPT},
		{"codex-auto-review", maxOutGPT},
		{"gpt-4o-mini", 0}, // pre-5 GPT slugs are outside the verified gate
		{"openai/gpt-4o", 0},
		{"glm-5.3", maxOutGLM},
		{"glm-5.2", maxOutGLM},
		{"glm-5.1", maxOutGLM},
		{"glm-5", maxOutGLM},
		{"glm-5-turbo", maxOutGLM},
		{"glm-4.7-flash", maxOutGLM},
		{"glm-4.6", maxOutGLM},
		{"some-unknown-model", 0},               // unknown → 0 (caller falls back)
		{"  Claude-Haiku  ", maxOutClaudeSmall}, // trimmed + case-insensitive
	}
	for _, c := range cases {
		if got := MaxOutputFor("", c.model); got != c.want {
			t.Errorf("MaxOutputFor(%q) = %d, want %d", c.model, got, c.want)
		}
	}
}

func TestCatalogFillsMaxOutput(t *testing.T) {
	for _, entry := range Catalog() {
		for _, m := range entry.Models {
			want := MaxOutputFor(entry.ID, m.ID)
			if m.MaxOutput != want {
				t.Errorf("catalog %s/%s MaxOutput = %d, want %d", entry.ID, m.ID, m.MaxOutput, want)
			}
		}
	}
}
