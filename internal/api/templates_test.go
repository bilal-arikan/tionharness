package api

import (
	"strings"
	"testing"

	"github.com/bilal-arikan/swarmgo/internal/market"
	"github.com/bilal-arikan/swarmgo/internal/orchestration"
)

// TestResolveTemplateFlowGraph covers the agent-key resolution for both the
// linear-steps and the full-graph (non-linear) template flow forms, plus the
// missing-key failure path.
func TestResolveTemplateFlowGraph(t *testing.T) {
	ids := map[string]string{"a": "AGT1", "b": "AGT2"}

	// Linear steps → sequential agent graph with ids wired.
	lin := market.WorkspaceTemplateFlow{
		Name: "lin",
		Steps: []market.WorkspaceTemplateStep{
			{ID: "s1", Title: "One", AgentKey: "a", Prompt: "{{input}}"},
			{ID: "s2", Title: "Two", AgentKey: "b", Prompt: "{{last}}"},
		},
	}
	g, ok := resolveTemplateFlowGraph(lin, ids)
	if !ok {
		t.Fatal("linear flow failed to resolve")
	}
	if g.Start != "s1" || len(g.Nodes) != 2 || g.Nodes[0].AgentID != "AGT1" || g.Nodes[1].AgentID != "AGT2" {
		t.Fatalf("unexpected linear graph: %+v", g)
	}

	// Non-linear graph (branch) with tmpl:<key> agent ids → substituted + valid.
	branchJSON := `{"start":"route","nodes":[` +
		`{"id":"route","type":"branch","branches":[{"contains":"yes","next":"yo"},{"contains":"","next":"no"}]},` +
		`{"id":"yo","type":"agent","agentId":"tmpl:a","prompt":"{{input}}"},` +
		`{"id":"no","type":"agent","agentId":"tmpl:b","prompt":"{{input}}"}]}`
	bg, ok := resolveTemplateFlowGraph(market.WorkspaceTemplateFlow{Name: "branch", Graph: branchJSON}, ids)
	if !ok {
		t.Fatal("branch flow failed to resolve")
	}
	for _, n := range bg.Nodes {
		if n.Type == orchestration.NodeAgent && strings.HasPrefix(n.AgentID, market.TemplateAgentKeyPrefix) {
			t.Fatalf("unsubstituted agent id: %q", n.AgentID)
		}
	}

	// A referenced agent key that does not resolve → failure.
	bad := market.WorkspaceTemplateFlow{Name: "bad", Graph: `{"start":"x","nodes":[{"id":"x","type":"agent","agentId":"tmpl:missing","prompt":"hi"}]}`}
	if _, ok := resolveTemplateFlowGraph(bad, ids); ok {
		t.Fatal("expected failure for missing agent key")
	}
}

// TestBundledTemplatePacksIntegrity verifies every bundled workspace-template
// pack is internally consistent: it carries a workspace payload, unique flow step
// ids, all flow/schedule agent keys resolve to a declared agent, and the
// resulting orchestration graph validates. This guards the embedded packs the
// create-workspace picker seeds from.
func TestBundledTemplatePacksIntegrity(t *testing.T) {
	// Bundled-only store (empty global dir): exactly the embedded template packs.
	store := market.New("", "")
	packs := store.ListKind(market.KindWorkspace)
	if len(packs) == 0 {
		t.Fatal("no bundled workspace template packs found")
	}

	sawBlank := false
	for _, meta := range packs {
		full, ok := store.Get(meta.ID)
		if !ok {
			t.Fatalf("%s: Get failed", meta.ID)
		}
		t.Run(meta.ID, func(t *testing.T) {
			wp := full.Payload.Workspace
			if wp == nil {
				t.Fatal("workspace pack has no workspace payload")
			}
			if meta.ID == blankTemplateID {
				sawBlank = true
			}
			if len(wp.Agents) == 0 {
				t.Fatal("template has no agents")
			}
			keys := map[string]bool{}
			for _, a := range wp.Agents {
				if a.Key == "" {
					t.Fatal("agent with empty key")
				}
				if keys[a.Key] {
					t.Fatalf("duplicate agent key %q", a.Key)
				}
				keys[a.Key] = true
			}

			// Schedules must reference a declared agent.
			for _, sc := range wp.Schedules {
				if !keys[sc.AgentKey] {
					t.Fatalf("schedule references unknown agent key %q", sc.AgentKey)
				}
			}

			// Every flow (linear or graph) must resolve to a valid graph with the
			// placeholder agent ids — exactly what seeding does at runtime.
			ids := map[string]string{}
			for k := range keys {
				ids[k] = "agent-" + k
			}
			for _, tf := range wp.Flows {
				if _, ok := resolveTemplateFlowGraph(tf, ids); !ok {
					t.Fatalf("flow %q did not resolve to a valid graph", tf.Name)
				}
			}
		})
	}

	if !sawBlank {
		t.Fatalf("expected a bundled %q template", blankTemplateID)
	}
}
