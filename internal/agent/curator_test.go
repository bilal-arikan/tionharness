package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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
		for range 3 {
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
	// Load an aged persisted hook instead of exposing a production timestamp
	// mutator solely for this fixture.
	hook.CreatedAt = now - 40*24*3600
	hookData, err := json.Marshal(hook)
	if err != nil {
		t.Fatal(err)
	}
	storeRoot := rt.db.Root()
	if err := rt.db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storeRoot, "hooks", hook.ID+".json"), hookData, 0o644); err != nil {
		t.Fatal(err)
	}
	rt.db, err = db.Open(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.db.Close() })

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
