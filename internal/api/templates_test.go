package api

import (
	"testing"

	"github.com/bilal/swarmgo/internal/orchestration"
)

// TestWorkspaceTemplatesIntegrity verifies every template is internally
// consistent: unique flow step ids, all flow/schedule agent keys resolve to a
// declared agent, and the resulting orchestration graph validates.
func TestWorkspaceTemplatesIntegrity(t *testing.T) {
	for _, tmpl := range workspaceTemplates {
		t.Run(tmpl.ID, func(t *testing.T) {
			if len(tmpl.Agents) == 0 {
				t.Fatal("template has no agents")
			}
			keys := map[string]bool{}
			for _, a := range tmpl.Agents {
				if a.Key == "" {
					t.Fatal("agent with empty key")
				}
				if keys[a.Key] {
					t.Fatalf("duplicate agent key %q", a.Key)
				}
				keys[a.Key] = true
			}

			// Schedules must reference a declared agent.
			for _, s := range tmpl.Schedules {
				if !keys[s.AgentKey] {
					t.Fatalf("schedule references unknown agent key %q", s.AgentKey)
				}
			}

			if tmpl.Flow == nil {
				return
			}

			// Build a graph with placeholder agent IDs and validate it the same
			// way seedTemplateFlow would after key→ID resolution.
			ids := map[string]string{}
			for k := range keys {
				ids[k] = "agent-" + k
			}
			seen := map[string]bool{}
			nodes := make([]orchestration.Node, 0, len(tmpl.Flow.Steps))
			for i, st := range tmpl.Flow.Steps {
				if st.ID == "" {
					t.Fatal("flow step with empty id")
				}
				if seen[st.ID] {
					t.Fatalf("duplicate flow step id %q", st.ID)
				}
				seen[st.ID] = true
				if !keys[st.AgentKey] {
					t.Fatalf("flow step %q references unknown agent key %q", st.ID, st.AgentKey)
				}
				next := ""
				if i+1 < len(tmpl.Flow.Steps) {
					next = tmpl.Flow.Steps[i+1].ID
				}
				nodes = append(nodes, orchestration.Node{
					ID: st.ID, Type: orchestration.NodeAgent, Title: st.Title,
					AgentID: ids[st.AgentKey], Prompt: st.Prompt, Next: next,
				})
			}
			g := orchestration.Graph{Start: tmpl.Flow.Steps[0].ID, Nodes: nodes}
			if err := g.Validate(); err != nil {
				t.Fatalf("flow graph invalid: %v", err)
			}
		})
	}
}

// TestTemplateByIDFallback verifies an unknown id falls back to "blank".
func TestTemplateByIDFallback(t *testing.T) {
	if got := templateByID("does-not-exist").ID; got != "blank" {
		t.Fatalf("expected blank fallback, got %q", got)
	}
	if got := templateByID("research").ID; got != "research" {
		t.Fatalf("expected research, got %q", got)
	}
}
