package agent

import (
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/skills"
)

func testRecipeSpec() *skills.RecipeSpec {
	return &skills.RecipeSpec{
		Version: "3",
		Phases: []skills.PhaseSpec{
			{ID: "plan", Profile: "planner", Gate: &skills.GateSpec{Kind: "artifact", Value: "plan"}},
			{ID: "code", Profile: "coder", Watchers: []string{"summarize-board"}},
			{ID: "review", Label: "Review", Profile: "validator", Optional: true},
		},
		Watchers:  []string{"update-docs"},
		Optimizer: "recipe-optimizer",
	}
}

// TestTrajSeedFromRecipe: the seed carries the root node on lane 0, the phases
// next-chained in order with profile/gate/optional, per-phase and global
// watchers as ghost automations, and the optimizer.
func TestTrajSeedFromRecipe(t *testing.T) {
	root := db.Session{ID: "SES1", Title: "Root", CreatedAt: 100}
	tr := trajSeed(root, testRecipeSpec(), "plan-dev@3")
	if err := (db.Trajectory{RootSessionID: "SES1", Status: tr.Status, Nodes: tr.Nodes, Edges: tr.Edges}).Validate(); err != nil {
		t.Fatalf("seed invalid: %v", err)
	}
	if tr.TemplateRef != "plan-dev@3" || tr.Status != db.TrajStatusPlanned {
		t.Fatalf("seed header = %+v", tr)
	}
	rootNode := trajNodePtr(&tr, "s:SES1")
	if rootNode == nil || rootNode.Lane != 0 || rootNode.State != db.TrajStateActive || rootNode.StartMs != 100_000 {
		t.Fatalf("root node = %+v", rootNode)
	}
	phases := trajPhases(&tr)
	if len(phases) != 3 || phases[0].ID != "p:plan" || phases[1].ID != "p:code" || phases[2].ID != "p:review" {
		t.Fatalf("phases = %+v", phases)
	}
	if phases[0].Gate == nil || phases[0].Gate.Kind != "artifact" || phases[0].Profile != "planner" || phases[0].Label != "plan" {
		t.Fatalf("plan phase = %+v", phases[0])
	}
	if !phases[2].Optional || phases[2].Label != "Review" {
		t.Fatalf("review phase = %+v", phases[2])
	}
	var next int
	for _, e := range tr.Edges {
		if e.Kind == db.TrajEdgeNext && e.Origin == db.TrajOriginDeclared {
			next++
		}
	}
	if next != 2 {
		t.Fatalf("next edges = %d, want 2", next)
	}
	if n := trajNodePtr(&tr, "a:summarize-board@code"); n == nil || n.PhaseID != "p:code" || n.State != db.TrajStateGhost {
		t.Fatalf("phase watcher = %+v", n)
	}
	if n := trajNodePtr(&tr, "a:update-docs"); n == nil || n.PhaseID != "" || n.Kind != db.TrajNodeAutomation {
		t.Fatalf("global watcher = %+v", n)
	}
	if n := trajNodePtr(&tr, trajOptimizerNodeID); n == nil || n.RefID != "recipe-optimizer" {
		t.Fatalf("optimizer = %+v", n)
	}
	if bare := trajSeed(root, nil, ""); len(bare.Nodes) != 1 || bare.TemplateRef != "" {
		t.Fatalf("agent-planned seed = %+v", bare)
	}
}

// TestTrajPhaseTransitionsAndStatus: activating a phase closes the previous
// one; the status follows the phases (planned → running → done) and an open
// gate wins as waiting; optional phases do not hold "done" back.
func TestTrajPhaseTransitionsAndStatus(t *testing.T) {
	tr := trajSeed(db.Session{ID: "SES1"}, testRecipeSpec(), "r@3")
	trajDeriveStatus(&tr)
	if tr.Status != db.TrajStatusPlanned {
		t.Fatalf("status before start = %s", tr.Status)
	}
	trajAutoStartPhase(&tr, 10)
	if trajActivePhase(&tr) != "p:plan" || trajNodePtr(&tr, "p:plan").StartMs != 10 {
		t.Fatalf("auto start = %+v", trajPhases(&tr))
	}
	trajAutoStartPhase(&tr, 20) // no-op while one is active
	if trajActivePhase(&tr) != "p:plan" {
		t.Fatal("auto start must not move an active phase")
	}
	trajDeriveStatus(&tr)
	if tr.Status != db.TrajStatusRunning {
		t.Fatalf("status with active phase = %s", tr.Status)
	}
	if err := trajSetPhaseState(&tr, "code", db.TrajStateActive, "", 30); err != nil {
		t.Fatal(err)
	}
	if p := trajNodePtr(&tr, "p:plan"); p.State != db.TrajStateDone || p.EndMs != 30 {
		t.Fatalf("previous phase after activation = %+v", p)
	}
	if err := trajSetPhaseState(&tr, "nope", db.TrajStateActive, "", 30); err == nil {
		t.Fatal("unknown phase must be refused")
	}
	if err := trajSetPhaseState(&tr, "code", "bogus", "", 30); err == nil {
		t.Fatal("unknown state must be refused")
	}
	// A gate opens: waiting beats running.
	trajAddNode(&tr, db.TrajectoryNode{ID: "g:ASK1", Kind: db.TrajNodeGate, Origin: db.TrajOriginObserved, State: db.TrajStateActive})
	trajDeriveStatus(&tr)
	if tr.Status != db.TrajStatusWaiting {
		t.Fatalf("status with open gate = %s", tr.Status)
	}
	trajNodePtr(&tr, "g:ASK1").State = db.TrajStateDone
	if err := trajSetPhaseState(&tr, "code", db.TrajStateDone, "", 40); err != nil {
		t.Fatal(err)
	}
	trajDeriveStatus(&tr)
	if tr.Status != db.TrajStatusDone {
		t.Fatalf("status with only the optional phase pending = %s, want done", tr.Status)
	}
	// A failed phase fails the trajectory.
	tr2 := trajSeed(db.Session{ID: "SES2"}, testRecipeSpec(), "r@3")
	_ = trajSetPhaseState(&tr2, "plan", db.TrajStateFailed, "planner gave up", 5)
	trajDeriveStatus(&tr2)
	if tr2.Status != db.TrajStatusFailed || trajNodePtr(&tr2, "p:plan").Reason != "planner gave up" {
		t.Fatalf("failed phase → %s / %+v", tr2.Status, trajNodePtr(&tr2, "p:plan"))
	}
	// A terminal status is never re-derived away.
	tr2.Status = db.TrajStatusAbandoned
	trajDeriveStatus(&tr2)
	if tr2.Status != db.TrajStatusAbandoned {
		t.Fatalf("terminal status re-derived to %s", tr2.Status)
	}
}

// TestTrajApplyPlan: a plan declares phases with the recipe's id rule; a
// re-plan keeps the state of phases it retains, may add and drop pending ones,
// and is refused when it would drop an active/done phase.
func TestTrajApplyPlan(t *testing.T) {
	tr := trajSeed(db.Session{ID: "SES1"}, nil, "")
	if err := trajApplyPlan(&tr, nil); err == nil {
		t.Fatal("empty plan must be refused")
	}
	if err := trajApplyPlan(&tr, []TrajectoryPlanPhase{{ID: "Bad Id"}}); err == nil {
		t.Fatal("invalid id must be refused")
	}
	if err := trajApplyPlan(&tr, []TrajectoryPlanPhase{{ID: "a"}, {ID: "a"}}); err == nil {
		t.Fatal("duplicate id must be refused")
	}
	if err := trajApplyPlan(&tr, []TrajectoryPlanPhase{{ID: "a", GateKind: "magic"}}); err == nil {
		t.Fatal("unknown gate kind must be refused")
	}
	plan := []TrajectoryPlanPhase{
		{ID: "research", Profile: "explore"},
		{ID: "build", Label: "Build it", Profile: "coder", GateKind: "verdict", GateVal: "PASS"},
		{ID: "docs", Optional: true},
	}
	if err := trajApplyPlan(&tr, plan); err != nil {
		t.Fatal(err)
	}
	phases := trajPhases(&tr)
	if len(phases) != 3 || phases[1].Label != "Build it" || phases[1].Gate == nil || phases[1].Gate.Value != "PASS" || !phases[2].Optional {
		t.Fatalf("declared phases = %+v", phases)
	}
	if tr.Nodes[0].ID != "p:research" || tr.Nodes[3].ID != "s:SES1" {
		t.Fatalf("phases must lead the node list: %+v", tr.Nodes)
	}
	if err := tr.Validate(); err != nil {
		// Validate needs the store-owned fields; fake them.
		tr.RootSessionID, tr.Status = "SES1", db.TrajStatusPlanned
		if err := tr.Validate(); err != nil {
			t.Fatalf("plan produced an invalid graph: %v", err)
		}
	}
	_ = trajSetPhaseState(&tr, "research", db.TrajStateActive, "", 1)
	// Hang a worker under the active phase; a re-plan that drops that phase is refused.
	trajAddNode(&tr, db.TrajectoryNode{ID: "s:W1", Kind: db.TrajNodeSession, Origin: db.TrajOriginObserved, PhaseID: "p:research", Lane: 1, State: db.TrajStateActive})
	if err := trajApplyPlan(&tr, []TrajectoryPlanPhase{{ID: "build"}}); err == nil || !strings.Contains(err.Error(), "research") {
		t.Fatalf("dropping the active phase must be refused, got %v", err)
	}
	// Dropping the pending "docs" and adding "ship" is fine; research stays active.
	if err := trajApplyPlan(&tr, []TrajectoryPlanPhase{{ID: "research"}, {ID: "build"}, {ID: "ship"}}); err != nil {
		t.Fatal(err)
	}
	if trajNodePtr(&tr, "p:docs") != nil || trajNodePtr(&tr, "p:ship") == nil {
		t.Fatalf("re-plan nodes = %+v", trajPhases(&tr))
	}
	if trajActivePhase(&tr) != "p:research" || trajNodePtr(&tr, "s:W1").PhaseID != "p:research" {
		t.Fatal("re-plan must keep the retained phase's state and bindings")
	}
	var next []string
	for _, e := range tr.Edges {
		if e.Kind == db.TrajEdgeNext {
			next = append(next, e.From+">"+e.To)
		}
	}
	if strings.Join(next, ",") != "p:research>p:build,p:build>p:ship" {
		t.Fatalf("next chain = %v", next)
	}
}

// TestTrajRender: the text view names status, phases with glyphs, workers under
// their phase and open gates.
func TestTrajRender(t *testing.T) {
	tr := trajSeed(db.Session{ID: "SES1", Title: "Root"}, testRecipeSpec(), "plan-dev@3")
	tr.ID, tr.Revision, tr.Status = "RTA7", 4, db.TrajStatusRunning
	trajAutoStartPhase(&tr, 1)
	trajAddNode(&tr, db.TrajectoryNode{ID: "s:W1", Kind: db.TrajNodeSession, Origin: db.TrajOriginObserved, Label: "Planner", RefID: "W1", PhaseID: "p:plan", Lane: 1, State: db.TrajStateDone})
	trajAddNode(&tr, db.TrajectoryNode{ID: "g:ASK1", Kind: db.TrajNodeGate, Origin: db.TrajOriginObserved, Label: "ask", RefID: "ASK1", Lane: 0, State: db.TrajStateActive})
	out := trajRender(&tr)
	for _, want := range []string{"Trajectory RTA7", "status running", "revision 4", "recipe plan-dev@3",
		"plan ● (planner, gate artifact \"plan\")", "review ○ (validator, optional)", "Under plan:", "worker W1 [done] Planner", "Open gates", "ASK1"} {
		if !strings.Contains(out, want) {
			t.Fatalf("render missing %q:\n%s", want, out)
		}
	}
	bare := trajSeed(db.Session{ID: "SES2"}, nil, "")
	if out := trajRender(&bare); !strings.Contains(out, "none declared") {
		t.Fatalf("bare render = %s", out)
	}
}
