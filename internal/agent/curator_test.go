package agent

import (
	"context"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// TestCuratorArchivesAgentMadeAndSuggestsUserMade: exhausted / expired rules
// and schedules are archived when an agent made them and only suggested when
// the user did; pinned ones are untouched; a quiet hook is a suggestion; the
// report is saved and drives the weekly clock.
func TestCuratorArchivesAgentMadeAndSuggestsUserMade(t *testing.T) {
	rt := lifecycleRuntime(t)
	ctx := context.Background()
	now := time.Now().Unix()
	mk := func(a db.Automation) db.Automation {
		a.PromptTemplate, a.TriggerTag, a.TargetAgentID, a.Enabled = "go", "loop", "AGT1", true
		if a.MaxIterations == 0 {
			a.MaxIterations = 5
		}
		created, err := rt.db.CreateAutomation(ctx, a)
		if err != nil {
			t.Fatal(err)
		}
		return created
	}
	exhaustedAgent := mk(db.Automation{Name: "bot-loop", CreatedBy: "AGT1", MaxIterations: 3})
	exhaustedUser := mk(db.Automation{Name: "my-loop", MaxIterations: 3})
	pinned := mk(db.Automation{Name: "keep", CreatedBy: "AGT1", MaxIterations: 3})
	expiredAgent := mk(db.Automation{Name: "old", CreatedBy: "automation:AUT1", ExpiresAt: now - 10})
	healthy := mk(db.Automation{Name: "fine", CreatedBy: "AGT1"})
	for _, a := range []db.Automation{exhaustedAgent, exhaustedUser, pinned} {
		for i := 0; i < 3; i++ {
			if err := rt.db.RecordAutomationFire(ctx, a.ID, "SESx", ""); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := rt.db.SetAutomationPinned(ctx, pinned.ID, true); err != nil {
		t.Fatal(err)
	}
	sc, err := rt.db.CreateSchedule(ctx, db.Schedule{Name: "once", AgentID: "AGT1", CronExpr: "* * * * *", Prompt: "p", OneShot: true, CreatedBy: "AGT1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.db.SetScheduleDelivery(ctx, sc.ID, "ok", "", 0); err != nil {
		t.Fatal(err)
	}
	if err := rt.db.SetScheduleEnabled(ctx, sc.ID, false); err != nil {
		t.Fatal(err)
	}
	hook, err := rt.db.CreateHook(ctx, db.Hook{Event: "PreToolUse", Matcher: "shell", Type: "command", Command: "true", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.db.SetHookCreatedAtForTest(ctx, hook.ID, now-40*24*3600); err != nil {
		t.Fatal(err)
	}

	rep, err := rt.RunCurator(ctx, "manual", true)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]db.CuratorAction{}
	for _, a := range rep.Actions {
		byID[a.ID] = a
	}
	if a := byID[exhaustedAgent.ID]; !a.Applied || a.Kind != db.CuratorActionArchive || a.Reason != CuratorReasonExhausted {
		t.Fatalf("agent-made exhausted rule = %+v", a)
	}
	if a := byID[exhaustedUser.ID]; a.Applied || a.Kind != db.CuratorActionSuggest || a.Reason != CuratorReasonExhausted {
		t.Fatalf("user-made exhausted rule = %+v", a)
	}
	if _, ok := byID[pinned.ID]; ok {
		t.Fatal("a pinned rule must be exempt")
	}
	if a := byID[expiredAgent.ID]; !a.Applied || a.Reason != CuratorReasonExpired {
		t.Fatalf("expired rule = %+v", a)
	}
	if _, ok := byID[healthy.ID]; ok {
		t.Fatal("a healthy rule must not be touched")
	}
	if a := byID[sc.ID]; !a.Applied || a.Entity != db.CuratorEntitySchedule || a.Reason != CuratorReasonOneShotDone {
		t.Fatalf("one-shot schedule = %+v", a)
	}
	if a := byID[hook.ID]; a.Applied || a.Entity != db.CuratorEntityHook || a.Reason != CuratorReasonNeverFired {
		t.Fatalf("quiet hook = %+v", a)
	}
	got, _ := rt.db.GetAutomation(ctx, exhaustedAgent.ID)
	if !got.Archived {
		t.Fatal("agent-made exhausted rule must be archived on disk")
	}
	got, _ = rt.db.GetAutomation(ctx, exhaustedUser.ID)
	if got.Archived {
		t.Fatal("user-made rule must stay")
	}
	if rep.Archived != 3 || rep.Suggestions != 2 {
		t.Fatalf("report counts = %d archived, %d suggestions (%+v)", rep.Archived, rep.Suggestions, rep.Actions)
	}
	saved, ok, err := rt.db.GetCuratorReport(ctx)
	if err != nil || !ok || saved.At != rep.At || len(saved.Actions) != len(rep.Actions) {
		t.Fatalf("saved report = %+v / %v / %v", saved, ok, err)
	}
	// The weekly clock: a fresh report blocks the tick; a stale one lets it run
	// (the bare runtime reports idle=false, so the tick must still refuse).
	if rt.curatorTick(ctx, time.Now()) {
		t.Fatal("tick right after a pass must not run again")
	}
}

// TestCuratorRecipeSuggestions: a watcher unfired in every summarized run and
// a phase never reached in every run become recipe suggestions once three
// runs exist; fewer runs say nothing.
func TestCuratorRecipeSuggestions(t *testing.T) {
	rt := lifecycleRuntime(t)
	ctx := context.Background()
	ag, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "A", Provider: "anthropic"})
	mkRun := func(unfired []string, ghost []string) {
		root, _ := rt.db.CreateSession(ctx, db.Session{AgentID: ag.ID, CoordinatorMode: true})
		tr, err := rt.db.CreateTrajectory(ctx, db.Trajectory{RootSessionID: root.ID, TemplateRef: "plan-dev@3", Status: db.TrajStatusPlanned})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := rt.db.UpdateTrajectory(ctx, tr.ID, 0, func(t *db.Trajectory) error {
			t.Status = db.TrajStatusDone
			t.Summary = &db.TrajectorySummary{Priced: true, UnfiredWatchers: unfired, GhostPhases: ghost}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	mkRun([]string{"docs"}, []string{"ship"})
	mkRun([]string{"docs"}, nil)
	rep, _ := rt.RunCurator(ctx, "manual", false)
	if len(rep.Actions) != 0 {
		t.Fatalf("two runs must not yield recipe suggestions, got %+v", rep.Actions)
	}
	mkRun([]string{"docs", "lint"}, []string{"ship"})
	rep, _ = rt.RunCurator(ctx, "manual", false)
	var docs, lint, ship bool
	for _, a := range rep.Actions {
		if a.Entity != db.CuratorEntityRecipe || a.ID != "plan-dev" {
			continue
		}
		switch {
		case a.Reason == CuratorReasonUnfiredWatcher && contains(a.Detail, `"docs"`):
			docs = true
		case a.Reason == CuratorReasonUnfiredWatcher && contains(a.Detail, `"lint"`):
			lint = true
		case a.Reason == CuratorReasonGhostPhase && contains(a.Detail, `"ship"`):
			ship = true
		}
	}
	if !docs || lint || ship {
		t.Fatalf("recipe suggestions: docs=%v lint=%v ship=%v (%+v)", docs, lint, ship, rep.Actions)
	}
}

func contains(s, sub string) bool { return len(sub) > 0 && len(s) >= len(sub) && indexOf(s, sub) >= 0 }

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
