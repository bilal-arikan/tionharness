package providers

import "testing"

func TestRequiresAdaptiveThinking(t *testing.T) {
	adaptive := []string{"claude-fable-5", "Claude-Fable-5", "mythos-5-preview"}
	for _, m := range adaptive {
		if !RequiresAdaptiveThinking(m) {
			t.Errorf("RequiresAdaptiveThinking(%q) = false, want true", m)
		}
	}
	classic := []string{"claude-opus-4-8", "claude-sonnet-4-6", "claude-haiku-4-5", "gpt-4o", ""}
	for _, m := range classic {
		if RequiresAdaptiveThinking(m) {
			t.Errorf("RequiresAdaptiveThinking(%q) = true, want false", m)
		}
	}
}
