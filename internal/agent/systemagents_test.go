package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/prompts"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// TestInsightApplierAllowlistExcludesFilesAndConfig locks the boundary that makes
// the applier safe to point at a workspace: it edits workspace ENTITIES only. The
// allowlist is the enforcement — granting group:files or group:config would hand
// it Read/Write/Edit/Bash or settings/secret/workspace tools.
func TestInsightApplierAllowlistExcludesFilesAndConfig(t *testing.T) {
	def, ok := SystemAgentDefault("insight-applier")
	if !ok {
		t.Fatal("insight-applier default not found")
	}
	var allowed []string
	if err := json.Unmarshal([]byte(def.AllowedTools), &allowed); err != nil {
		t.Fatalf("allowedTools is not a JSON array: %v", err)
	}
	got := map[string]bool{}
	for _, name := range allowed {
		if strings.HasPrefix(name, tools.GroupPrefix) && !tools.ValidGroupKey(name) {
			t.Errorf("allowlist entry %q names no known tool category", name)
		}
		got[name] = true
	}
	for _, want := range []string{
		"group:automation", "group:agents", "group:skills-mcp", "group:artifacts",
		"insight_list_findings", "insight_apply_finding", "todo_write",
	} {
		if !got[want] {
			t.Errorf("allowlist is missing %q", want)
		}
	}
	for _, banned := range []string{"group:files", "group:config"} {
		if got[banned] {
			t.Errorf("allowlist must not grant %q", banned)
		}
	}
	if len(allowed) != 7 {
		t.Errorf("allowlist has %d entries, want exactly the 7 documented ones: %v", len(allowed), allowed)
	}
}

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
		{key: "recipe-optimizer", promptKey: "recipe-optimizer"},
		{key: "stall-judge", promptKey: "stall-judge"},
		{key: "insight-applier", promptKey: "insight-applier"},
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

// TestSystemAgentKeysGolden is the golden list of built-in system agent keys.
// Adding or renaming one must update this list, the prompt-registry ownership
// (prompts_test.go TestSystemAgentOwnership), _Docs/74 and, when the agent is a
// worker profile, defaultSubagentProfiles — the failure message says so.
func TestSystemAgentKeysGolden(t *testing.T) {
	want := []string{
		"titler", "overview-summarizer", "compaction", "lesson-extractor", "insight",
		"recipe-optimizer", "stall-judge", "insight-applier",
		"subagent-explore", "subagent-planner", "subagent-coder", "subagent-reviewer",
		"subagent-validator", "subagent-config",
	}
	defs := SystemAgentDefaults()
	if len(defs) != len(want) {
		t.Fatalf("system agent count = %d, want %d — update the golden list, prompts_test ownership map and _Docs/74", len(defs), len(want))
	}
	for i, def := range defs {
		if def.SystemKey != want[i] {
			t.Fatalf("system agent %d = %q, want %q (golden order)", i, def.SystemKey, want[i])
		}
		if def.Avatar == "" || def.Color == "" {
			t.Fatalf("system agent %q has no visual identity", def.SystemKey)
		}
	}
}
