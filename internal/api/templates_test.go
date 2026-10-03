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
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// TestBundledTemplatePacksIntegrity verifies every bundled workspace-template
// pack is internally consistent: it carries a workspace payload and all
// schedule/automation agent keys resolve to a declared agent. This guards the embedded packs the
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

			// Automations must reference a declared agent key, since
			// seedTemplateAutomations SKIPS the ones that do not resolve — a pack
			// shipping a dangling reference would silently install one rule short.
			for _, au := range wp.Automations {
				if au.AgentKey != "" && !keys[au.AgentKey] {
					t.Fatalf("automation %q references unknown agent key %q", au.Name, au.AgentKey)
				}
				if au.AgentKey == "" && au.BoardAction != db.BoardActionArchive {
					t.Fatalf("automation %q has no agent to run", au.Name)
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
	catalog, _ := runtime.WorkspaceToolCatalogWithState(context.Background())
	for _, def := range catalog {
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
