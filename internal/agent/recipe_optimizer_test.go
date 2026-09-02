package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/insight"
	"github.com/bilal-arikan/tionharness/internal/skills"
)

// TestOptimizerFindingInvariants: a proposal needs measured evidence, may not
// carry a generic negative judgement, must name a removal when it would push
// the recipe over the growth budget, and must use a known action.
func TestOptimizerFindingInvariants(t *testing.T) {
	ok := func(p rawProposal, budget int) bool {
		_, accepted := optimizerFinding("plan-dev", "3", p, budget, 100, []string{"SES1"})
		return accepted
	}
	good := rawProposal{Action: "make_optional", Target: "ship", Title: "ship fazını isteğe bağlı yap", Rationale: "hiç başlamıyor", Evidence: "ship unreached in 4/4 runs (RTA1, RTA2)", Severity: "med"}
	if !ok(good, 5) {
		t.Fatal("a measured pruning proposal must pass")
	}
	if ok(rawProposal{Action: "make_optional", Target: "ship", Title: "x", Evidence: ""}, 5) {
		t.Fatal("no evidence must be dropped")
	}
	if ok(rawProposal{Action: "make_optional", Target: "ship", Title: "x", Evidence: "it felt slow"}, 5) {
		t.Fatal("evidence without a number must be dropped")
	}
	if ok(rawProposal{Action: "change_profile", Target: "review", Title: "validator güvenilmez", Evidence: "3/3 runs"}, 5) {
		t.Fatal("a generic negative judgement must be dropped")
	}
	if ok(rawProposal{Action: "teleport", Target: "x", Title: "x", Evidence: "3/3"}, 5) {
		t.Fatal("an unknown action must be dropped")
	}
	add := rawProposal{Action: "add_gate", Target: "review", Value: "verdict PASS", Title: "review'a kapı ekle", Evidence: "review failed workers 2.0/run over 3 runs"}
	if ok(add, skills.RecipeGrowthBudget) {
		t.Fatal("an addition at the budget without a removal must be dropped")
	}
	add.Removes = "watcher docs"
	if !ok(add, skills.RecipeGrowthBudget) {
		t.Fatal("an addition naming its removal must pass")
	}
	if !ok(add, 2) {
		t.Fatal("an addition under the budget passes without a removal")
	}
	f, _ := optimizerFinding("plan-dev", "3", good, 5, 100, []string{"SES1", "SES2"})
	if f.Channel != insight.ChannelRecipeOpt || f.Signature != "recipe-opt:plan-dev:make_optional:ship" || f.FilePointer != "skill/plan-dev" ||
		f.Proposal == nil || f.Proposal.Action != "make_optional" || f.Proposal.Evidence != good.Evidence || len(f.EvidenceSessionIDs) != 2 || f.Status != insight.StatusNew {
		t.Fatalf("finding shape = %+v", f)
	}
}

func TestRecipeGrowthBudget(t *testing.T) {
	spec := &skills.RecipeSpec{Phases: []skills.PhaseSpec{{ID: "a", Watchers: []string{"w1", "w2"}}, {ID: "b"}}, Watchers: []string{"w3"}}
	if spec.GrowthUsed() != 5 || recipeBudgetUsed(spec) != 5 {
		t.Fatalf("growth used = %d / %d", spec.GrowthUsed(), recipeBudgetUsed(spec))
	}
	if err := spec.Validate(); err != nil {
		t.Fatalf("within budget must validate: %v", err)
	}
	for i := 0; i < skills.RecipeGrowthBudget; i++ {
		spec.Watchers = append(spec.Watchers, "w")
	}
	if err := spec.Validate(); err == nil || !strings.Contains(err.Error(), "growth budget") {
		t.Fatalf("over budget must fail validation, got %v", err)
	}
}

func TestOptimizerPromptCarriesEvidence(t *testing.T) {
	sk := skills.Skill{Slug: "plan-dev", Version: "3", Recipe: &skills.RecipeSpec{Phases: []skills.PhaseSpec{{ID: "plan", Profile: "planner"}, {ID: "ship", Optional: true}}, Watchers: []string{"docs"}}}
	stats := []RecipeStats{{TemplateRef: "plan-dev@3", Slug: "plan-dev", Version: "3", Runs: 3, Done: 2, Failed: 1, Summarized: 3, AvgDurationSec: 900, AvgTokens: 40000, UnfiredWatchers: map[string]int{"docs": 3}, GhostPhases: map[string]int{"ship": 3}}}
	recent := []db.TrajectoryIndexEntry{{ID: "RTA9", TemplateRef: "plan-dev@3", Status: db.TrajStatusDone, Summary: &db.TrajectorySummary{DurationSec: 600, Tokens: 1000, Sessions: 2, GhostPhases: []string{"ship"}, UnfiredWatchers: []string{"docs"}, PerPhase: map[string]db.PhaseStat{"plan": {Sessions: 2, Failed: 1, DurationSec: 300}}}}}
	out := optimizerUserPrompt(sk, "# body", stats, recent, []string{"izleyici \"docs\" hiç ateşlenmedi (3 koşu)"}, "tr")
	for _, want := range []string{"Growth budget: 3 of 9", "- plan · profile planner", "- ship · optional", "recipe watchers: docs", "watcher docs unfired in 3/3", "phase ship unreached in 3/3", "RTA9 (plan-dev@3, done): 10m", "plan: 2 workers/1 failed/5m", "Curator (deterministic)", "in tr"} {
		if !strings.Contains(out, want) {
			t.Fatalf("prompt missing %q:\n%s", want, out)
		}
	}
	if got := recentSummaries(recent, "plan-dev", 5); len(got) != 1 || got[0].ID != "RTA9" {
		t.Fatalf("recentSummaries = %+v", got)
	}
	if got := recentSummaries(recent, "other", 5); len(got) != 0 {
		t.Fatalf("other slug = %+v", got)
	}
}

// TestRunRecipeOptimizerWithoutAgentRecordsSkip: the pass is bookkept even
// when it cannot call a model (no agent), so the threshold clock still moves
// and the API can show why nothing was proposed.
func TestRunRecipeOptimizerWithoutAgentRecordsSkip(t *testing.T) {
	workDir := filepath.Join(t.TempDir(), "workspace")
	skillDir := filepath.Join(workspaceSkillsDir(workDir), "plan-dev-test")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(testRecipeSkill), 0o644); err != nil {
		t.Fatal(err)
	}
	rt, _ := newTestRuntime(t, workDir)
	ctx := context.Background()
	// No runs yet → skipped for lack of evidence, state still written.
	res, err := rt.RunRecipeOptimizer(ctx, "plan-dev-test", "manual")
	if err != nil || res.Ran || res.Skipped != "no summarized runs yet" {
		t.Fatalf("empty pass = %+v / %v", res, err)
	}
	st, _ := rt.db.GetOptimizerState(ctx)
	if s := st.Slugs["plan-dev-test"]; s.Skipped != "no summarized runs yet" || s.Trigger != "manual" {
		t.Fatalf("state = %+v", s)
	}
	// Unknown recipe → error.
	if _, err := rt.RunRecipeOptimizer(ctx, "nope", "manual"); err == nil {
		t.Fatal("unknown recipe must error")
	}
	// With summarized runs but no agent in the store → "no agent available".
	root, _ := rt.db.CreateSession(ctx, db.Session{AgentID: "AGT-none", Title: "r", CoordinatorMode: true})
	tr, _ := rt.db.CreateTrajectory(ctx, db.Trajectory{RootSessionID: root.ID, TemplateRef: "plan-dev-test@3"})
	_, _ = rt.db.UpdateTrajectory(ctx, tr.ID, 0, func(t *db.Trajectory) error {
		t.Status = db.TrajStatusDone
		t.Summary = &db.TrajectorySummary{Priced: true}
		return nil
	})
	res, err = rt.RunRecipeOptimizer(ctx, "plan-dev-test", "manual")
	if err != nil || res.Ran || res.Skipped != "no agent available" {
		t.Fatalf("no-agent pass = %+v / %v", res, err)
	}
	st, _ = rt.db.GetOptimizerState(ctx)
	if s := st.Slugs["plan-dev-test"]; s.RunsSeen != 1 {
		t.Fatalf("runsSeen = %d", s.RunsSeen)
	}
}
