package agent

import (
	"encoding/json"
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
		{key: "subagent-explore", promptKey: "subagent-explore"},
		{key: "subagent-planner", promptKey: "subagent-planner"},
		{key: "subagent-coder", promptKey: "subagent-coder"},
		{key: "subagent-reviewer", promptKey: "subagent-reviewer"},
		{key: "subagent-validator", promptKey: "subagent-validator"},
		{key: "subagent-config", promptKey: "subagent-config"},
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

// TestSubagentSystemAgentDefaultsCarryProfileAllowlist keeps the built-in worker
// definitions bound to the code-side profile contract: the seeded agent row must
// start from exactly the profile's tools, never a hand-written copy.
func TestSubagentSystemAgentDefaultsCarryProfileAllowlist(t *testing.T) {
	for id, prof := range defaultSubagentProfiles {
		def, ok := SystemAgentDefault("subagent-" + id)
		if !ok {
			t.Fatalf("profile %q has no system agent default", id)
		}
		want, err := json.Marshal(prof.AllowedTools)
		if err != nil {
			t.Fatal(err)
		}
		if def.AllowedTools != string(want) {
			t.Errorf("profile %q allowlist = %s, want %s", id, def.AllowedTools, want)
		}
		if def.SuggestedModel != "" {
			t.Errorf("profile %q pins a model (%q); workers must inherit the coordinator's", id, def.SuggestedModel)
		}
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
