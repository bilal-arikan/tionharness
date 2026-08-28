package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/market"
	"github.com/bilal-arikan/tionharness/internal/orchestration"
	"github.com/bilal-arikan/tionharness/internal/providers"
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
	// A start node is prepended (required entry), so: start → s1(AGT1) → s2(AGT2).
	if g.Start != "start" || len(g.Nodes) != 3 {
		t.Fatalf("unexpected linear graph shape: %+v", g)
	}
	if g.Nodes[0].Type != orchestration.NodeStart || g.Nodes[0].Next != "s1" {
		t.Fatalf("first node should be the start node → s1: %+v", g.Nodes[0])
	}
	if g.Nodes[1].AgentID != "AGT1" || g.Nodes[2].AgentID != "AGT2" {
		t.Fatalf("agent ids not wired: %+v", g.Nodes)
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
			flowNames := map[string]bool{}
			for _, tf := range wp.Flows {
				if _, ok := resolveTemplateFlowGraph(tf, ids); !ok {
					t.Fatalf("flow %q did not resolve to a valid graph", tf.Name)
				}
				flowNames[tf.Name] = true
			}

			// Automations must reference a declared agent key / flow name, since
			// seedTemplateAutomations SKIPS the ones that do not resolve — a pack
			// shipping a dangling reference would silently install one rule short.
			for _, au := range wp.Automations {
				if au.AgentKey != "" && !keys[au.AgentKey] {
					t.Fatalf("automation %q references unknown agent key %q", au.Name, au.AgentKey)
				}
				if au.FlowName != "" && !flowNames[au.FlowName] {
					t.Fatalf("automation %q references unknown flow %q", au.Name, au.FlowName)
				}
				if au.AgentKey == "" && au.FlowName == "" && au.BoardAction != db.BoardActionArchive {
					t.Fatalf("automation %q has no agent or flow to run", au.Name)
				}
			}

			// A pinned coordinator recipe must be a skill the pack itself bundles or
			// one shipped as a built-in default — otherwise seeding drops it and the
			// agent quietly runs free coordination instead of the advertised recipe.
			bundled := map[string]bool{}
			for _, sk := range wp.Skills {
				bundled[sk.Slug] = true
			}
			for _, a := range wp.Agents {
				if a.CoordinatorWorkflow == "" {
					continue
				}
				if !a.CoordinatorMode {
					t.Fatalf("agent %q pins a coordinator recipe but is not a coordinator", a.Key)
				}
				if !bundled[a.CoordinatorWorkflow] && !strings.HasPrefix(a.CoordinatorWorkflow, "coordinator-wf-") {
					t.Fatalf("agent %q pins unbundled recipe %q", a.Key, a.CoordinatorWorkflow)
				}
			}
		})
	}

	if !sawBlank {
		t.Fatalf("expected a bundled %q template", blankTemplateID)
	}
}

func TestBlankTemplateSeedsCEOAndPMControlLoop(t *testing.T) {
	store := market.New("", "")
	pack, ok := store.Get(blankTemplateID)
	if !ok {
		t.Fatal("bundled blank template pack not found")
	}
	wp := pack.Payload.Workspace
	if wp == nil {
		t.Fatal("blank template pack has no workspace payload")
	}

	agents := make(map[string]market.WorkspaceTemplateAgent, len(wp.Agents))
	for _, a := range wp.Agents {
		agents[a.Key] = a
	}
	ceo, ok := agents["ceo"]
	if !ok {
		t.Fatal("blank template has no CEO agent")
	}
	if _, ok := agents["pm"]; !ok {
		t.Fatal("blank template has no PM agent")
	}
	var allowed []string
	if err := json.Unmarshal([]byte(ceo.AllowedTools), &allowed); err != nil {
		t.Fatalf("CEO allowedTools is invalid: %v", err)
	}
	allowedSet := make(map[string]bool, len(allowed))
	for _, name := range allowed {
		allowedSet[name] = true
	}
	var blocked []string
	if err := json.Unmarshal([]byte(ceo.BlockedTools), &blocked); err != nil {
		t.Fatalf("CEO blockedTools is invalid: %v", err)
	}
	blockedSet := make(map[string]bool, len(blocked))
	for _, name := range blocked {
		blockedSet[name] = true
	}
	for _, forbidden := range []string{
		"Write", "Edit", "Bash", "PowerShell",
		"create_task", "update_task", "delete_task", "move_task",
		"create_agent", "update_agent", "delete_agent",
		"create_flow", "update_flow", "delete_flow",
		"create_schedule", "update_schedule", "delete_schedule",
		"create_automation", "update_automation", "delete_automation",
		"toggle_mcp_server",
		// The PM is started as its own session, never as a worker of the CEO.
		"spawn_worker", "send_to_worker", "run_subagent",
	} {
		if !blockedSet[forbidden] {
			t.Errorf("CEO blockedTools is missing %q", forbidden)
		}
	}
	for _, required := range []string{"get_view", "list_tasks", "list_agents", "list_sessions", "send_message", "spawn_session"} {
		if !allowedSet[required] {
			t.Errorf("CEO allowedTools is missing %q", required)
		}
	}

	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open registry database: %v", err)
	}
	defer database.Close()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	runtime := agent.NewRuntime(database, providers.NewRegistry(), agent.NewTunables(), t.TempDir(), t.TempDir(), nil, nil, "WS1", "Test", nil, logger)
	defer runtime.CloseMCP()
	registered := make(map[string]bool)
	for _, def := range runtime.WorkspaceToolCatalog(context.Background()) {
		registered[def.Name] = true
	}
	// spawn_session is bound per session (it needs the spawn function), so it is
	// absent from the workspace-level catalog even though agents can call it.
	registered["spawn_session"] = true
	for _, name := range allowed {
		if !registered[name] {
			t.Errorf("CEO allowedTools contains unregistered tool %q", name)
		}
	}

	if len(wp.Schedules) != 1 {
		t.Fatalf("blank template schedules = %d, want 1", len(wp.Schedules))
	}
	schedule := wp.Schedules[0]
	if schedule.AgentKey != "ceo" || schedule.CronExpr != "7 * * * *" || !schedule.Enabled {
		t.Errorf("unexpected CEO schedule: %+v", schedule)
	}

	wantStates := map[string]bool{"failed": false, "review": false}
	for _, automation := range wp.Automations {
		if _, wanted := wantStates[automation.BoardToState]; !wanted {
			continue
		}
		if automation.TriggerKind != db.TriggerBoard || automation.BoardOp != db.BoardOpMove ||
			automation.AgentKey != "pm" || automation.SessionMode != db.SessionModeContinue || !automation.Enabled {
			t.Errorf("unexpected PM automation: %+v", automation)
			continue
		}
		if strings.TrimSpace(automation.PromptTemplate) == "" ||
			!strings.Contains(automation.PromptTemplate, "{{taskId}}") ||
			!strings.Contains(automation.PromptTemplate, "{{title}}") {
			t.Errorf("PM automation for %q has invalid promptTemplate %q", automation.BoardToState, automation.PromptTemplate)
		}
		wantStates[automation.BoardToState] = true
	}
	for state, found := range wantStates {
		if !found {
			t.Errorf("blank template has no enabled PM automation for %q", state)
		}
	}
}

// TestProductTeamTemplateShape locks the delegation chain the product-team pack
// exists to ship. Every assertion here is a way the pack could look installed-and-
// fine while being useless: a PM that is not a coordinator cannot hand work to the
// CTO at all, and a board rule that is not exclusive fires alongside the built-in
// default (seeded into every workspace with an EMPTY target) so one card move
// starts two runs, one of which immediately fails.
func TestProductTeamTemplateShape(t *testing.T) {
	store := market.New("", "")
	pack, ok := store.Get("workspace-product-team")
	if !ok {
		t.Fatal("bundled product-team template pack not found")
	}
	wp := pack.Payload.Workspace
	if wp == nil {
		t.Fatal("product-team pack has no workspace payload")
	}

	coordinators := map[string]bool{}
	for _, a := range wp.Agents {
		if a.CoordinatorMode {
			coordinators[a.Key] = true
		}
	}
	for _, key := range []string{"pm", "cto"} {
		if !coordinators[key] {
			t.Errorf("agent %q must ship as a coordinator — the whole point of the pack", key)
		}
	}

	if len(wp.Automations) == 0 {
		t.Fatal("expected the board-driven-execution rule to ship with the pack")
	}
	for _, au := range wp.Automations {
		if au.TriggerKind != db.TriggerBoard {
			continue
		}
		if !au.BoardExclusive {
			t.Errorf("board automation %q must be exclusive so it wins over the built-in default", au.Name)
		}
	}
}
