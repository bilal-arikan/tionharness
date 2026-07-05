package agent

import (
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/skills"
)

// TestSkillToolNameFor checks that claude-cli agents (including the empty default
// provider) get the namespaced Interaction-MCP identifier for use_skill, while
// native/custom providers get the bare name — so the Available Skills prompt block
// names the tool exactly as the agent will see it.
func TestSkillToolNameFor(t *testing.T) {
	ns := interactionToolPrefix + skills.DefaultSkillTool
	cases := []struct {
		provider string
		want     string
	}{
		{"", ns},               // empty → keyless claude-cli default
		{"claude-cli", ns},     // bridged through Interaction MCP
		{"anthropic", skills.DefaultSkillTool},
		{"minimax", skills.DefaultSkillTool},
		{"openrouter", skills.DefaultSkillTool},
		{"my-custom-openai", skills.DefaultSkillTool},
	}
	for _, c := range cases {
		if got := skillToolNameFor(c.provider); got != c.want {
			t.Errorf("skillToolNameFor(%q) = %q, want %q", c.provider, got, c.want)
		}
	}
}
