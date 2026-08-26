package agent

import (
	"testing"

	"github.com/bilal-arikan/tionharness/internal/prompts"
)

func TestSystemAgentDefaults(t *testing.T) {
	tests := []struct {
		key       string
		promptKey string
	}{
		{key: "titler", promptKey: "title"},
		{key: "overview-summarizer", promptKey: "summary"},
		{key: "compaction", promptKey: "compact"},
		{key: "lesson-extractor", promptKey: "lesson"},
		{key: "insight", promptKey: "insight-analyzer"},
	}
	if got := len(SystemAgentDefaults()); got != len(tests) {
		t.Fatalf("default count = %d, want %d", got, len(tests))
	}
	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			def, ok := SystemAgentDefault(tc.key)
			if !ok {
				t.Fatal("default not found")
			}
			if def.SystemPrompt != prompts.Default(tc.promptKey) {
				t.Fatal("system prompt differs from current runtime default")
			}
			if got, want := def.Disabled, tc.key == "insight"; got != want {
				t.Fatalf("disabled = %v, want %v", got, want)
			}
		})
	}
	if _, ok := SystemAgentDefault("missing"); ok {
		t.Fatal("unknown key unexpectedly found")
	}
}

func TestSystemAgentDefaultsReturnsCopy(t *testing.T) {
	defs := SystemAgentDefaults()
	defs[0].Name = "changed"
	got, _ := SystemAgentDefault(defs[0].SystemKey)
	if got.Name == "changed" {
		t.Fatal("registry mutated through list accessor")
	}
}
