package providers

import "testing"

func TestContextWindowFor(t *testing.T) {
	cases := []struct {
		model string
		want  int
	}{
		{"claude-opus-4-8", windowClaude},
		{"claude-haiku-4-5-20251001", windowClaude},
		{"anthropic/claude-sonnet-4.6", windowClaude},
		{"opus", windowClaude},
		{"sonnet", windowClaude},
		{"haiku", windowClaude},
		{"claude-fable-5", windowClaude},
		{"MiniMax-M3", windowMiniMax},
		{"minimax/minimax-m3", windowMiniMax},
		{"deepseek/deepseek-v4-flash", windowDeepSeek},
		{"google/gemini-3.5-flash", windowGemini},
		{"", 0},                          // claude-cli default → unknown
		{"openai/gpt-5.5", 0},            // not in a confident family → unknown
		{"some-unknown-model", 0},        // unknown → 0 (caller falls back)
		{"  Claude-Opus  ", windowClaude}, // trimmed + case-insensitive
	}
	for _, c := range cases {
		if got := ContextWindowFor("", c.model); got != c.want {
			t.Errorf("ContextWindowFor(%q) = %d, want %d", c.model, got, c.want)
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
