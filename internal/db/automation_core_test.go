package db

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// TestTriggerRegistryValidatesEveryKind pins the registry-backed shape check to
// the behaviour the old switch had, kind by kind, and rejects an unknown kind.
func TestTriggerRegistryValidatesEveryKind(t *testing.T) {
	kinds := TriggerKinds()
	for _, k := range []string{TriggerTag, TriggerBoard, TriggerToken} {
		found := false
		for _, r := range kinds {
			if r == k {
				found = true
			}
		}
		if !found {
			t.Fatalf("kind %q not registered (have %v)", k, kinds)
		}
	}
	base := Automation{PromptTemplate: "go", TargetAgentID: "AGT1", MaxIterations: 5}
	cases := []struct {
		name    string
		mutate  func(a *Automation)
		wantErr string
	}{
		{"legacy empty kind is tag and needs a tag", func(a *Automation) {}, "triggerTag is required"},
		{"tag ok", func(a *Automation) { a.TriggerTag = "x" }, ""},
		{"board needs valid op", func(a *Automation) { a.TriggerKind = TriggerBoard; a.BoardOp = "teleport" }, "invalid boardOp"},
		{"board archive needs no target/prompt", func(a *Automation) {
			a.TriggerKind, a.BoardAction, a.PromptTemplate, a.TargetAgentID = TriggerBoard, BoardActionArchive, "", ""
		}, ""},
		{"board move must not self-trigger", func(a *Automation) {
			a.TriggerKind, a.BoardAction, a.BoardMoveToState, a.BoardToState = TriggerBoard, BoardActionMove, "done", "done"
		}, "must differ"},
		{"token threshold floor", func(a *Automation) { a.TriggerKind, a.TokenScope, a.TokenThreshold = TriggerToken, "session", 10 }, "threshold"},
		{"retired counter kind", func(a *Automation) { a.TriggerKind = TriggerCounterLegacy }, "unknown triggerKind"},
		{"unknown kind", func(a *Automation) { a.TriggerKind = "teleport"; a.TriggerTag = "x" }, "unknown triggerKind"},
		// Rota (F2) kinds: registered like the others, with their own filters.
		{"phase rule ok", func(a *Automation) { a.TriggerKind, a.TrajPhase = TriggerPhase, "code" }, ""},
		{"phase rule bad event", func(a *Automation) { a.TriggerKind, a.TrajEvent = TriggerPhase, "during" }, "trajEvent"},
		{"phase rule multi-word phase", func(a *Automation) { a.TriggerKind, a.TrajPhase = TriggerPhase, "plan code" }, "trajPhase"},
		{"end rule ok", func(a *Automation) { a.TriggerKind, a.TrajStatus = TriggerTrajectoryEnd, TrajStatusFailed }, ""},
		{"end rule bad status", func(a *Automation) { a.TriggerKind, a.TrajStatus = TriggerTrajectoryEnd, "running" }, "trajStatus"},
		{"common: target required", func(a *Automation) { a.TriggerTag = "x"; a.TargetAgentID = "" }, "targetAgentId or flowId"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := base
			tc.mutate(&a)
			err := ValidateAutomationShape(a)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.wantErr)) {
				t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
	if ValidTriggerKind("teleport") || !ValidTriggerKind("") || !ValidTriggerKind(TriggerBoard) || !ValidTriggerKind(TriggerPhase) || !ValidTriggerKind(TriggerTrajectoryEnd) {
		t.Fatal("ValidTriggerKind mismatch")
	}
}

// TestAutomationFireLedger: append/list newest-first, cap + trim, clear on delete.
func TestAutomationFireLedger(t *testing.T) {
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer d.Close()
	ag, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	a, err := d.CreateAutomation(ctx, Automation{Name: "r", TriggerTag: "t", PromptTemplate: "p", TargetAgentID: ag.ID, MaxIterations: 5})
	if err != nil {
		t.Fatalf("create automation: %v", err)
	}
	if recs, err := d.ListAutomationFires(ctx, a.ID, 0); err != nil || len(recs) != 0 {
		t.Fatalf("empty ledger = %v err=%v", recs, err)
	}
	for i := 0; i < 3; i++ {
		if err := d.AppendAutomationFire(ctx, a.ID, AutomationFireRecord{Outcome: AutomationFireSkipped, Reason: AutomationSkipCooldown, Iteration: i}); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	if err := d.AppendAutomationFire(ctx, a.ID, AutomationFireRecord{Outcome: AutomationFireFired, SessionID: "SES9", Iteration: 4}); err != nil {
		t.Fatalf("append fired: %v", err)
	}
	recs, err := d.ListAutomationFires(ctx, a.ID, 2)
	if err != nil || len(recs) != 2 || recs[0].Outcome != AutomationFireFired || recs[0].SessionID != "SES9" || recs[1].Iteration != 2 || recs[0].At == 0 {
		t.Fatalf("list newest-first limit 2 = %+v err=%v", recs, err)
	}
	// Trim: past automationFireTrimAt lines only the newest automationFireCap stay.
	for i := 0; i < automationFireTrimAt; i++ {
		_ = d.AppendAutomationFire(ctx, a.ID, AutomationFireRecord{Outcome: AutomationFireSkipped, Iteration: 100 + i})
	}
	all, _ := d.ListAutomationFires(ctx, a.ID, 0)
	if len(all) != automationFireCap || all[0].Iteration != 100+automationFireTrimAt-1 {
		t.Fatalf("after trim: %d records, newest iteration %d; want %d / %d", len(all), all[0].Iteration, automationFireCap, 100+automationFireTrimAt-1)
	}
	if err := d.DeleteAutomation(ctx, a.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if recs, _ := d.ListAutomationFires(ctx, a.ID, 0); len(recs) != 0 {
		t.Fatal("delete must clear the ledger")
	}
}

// TestArchivedEntitiesLeaveTheActiveLists: archiving an automation, schedule or
// hook removes it from the engine-facing lists without deleting it, keeps
// Enabled as-is, and is reversible; hook fires are counted.
func TestArchivedEntitiesLeaveTheActiveLists(t *testing.T) {
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer d.Close()
	ag, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	a, _ := d.CreateAutomation(ctx, Automation{Name: "r", TriggerTag: "t", PromptTemplate: "p", TargetAgentID: ag.ID, MaxIterations: 5, Enabled: true})
	sc, _ := d.CreateSchedule(ctx, Schedule{Name: "s", AgentID: ag.ID, CronExpr: "* * * * *", Prompt: "hi", Enabled: true})
	h, _ := d.CreateHook(ctx, Hook{Event: HookPreToolUse, Type: "command", Command: "echo", Enabled: true})

	if err := d.SetAutomationArchived(ctx, a.ID, true); err != nil {
		t.Fatalf("archive automation: %v", err)
	}
	if err := d.SetScheduleArchived(ctx, sc.ID, true); err != nil {
		t.Fatalf("archive schedule: %v", err)
	}
	if err := d.SetHookArchived(ctx, h.ID, true); err != nil {
		t.Fatalf("archive hook: %v", err)
	}
	if list, _ := d.ListEnabledAutomations(ctx); len(list) != 0 {
		t.Fatalf("archived automation still enabled-listed: %+v", list)
	}
	if list, _ := d.ListEnabledSchedules(ctx); len(list) != 0 {
		t.Fatalf("archived schedule still enabled-listed: %+v", list)
	}
	if list, _ := d.ListEnabledHooksByEvent(ctx, HookPreToolUse); len(list) != 0 {
		t.Fatalf("archived hook still enabled-listed: %+v", list)
	}
	got, _ := d.GetAutomation(ctx, a.ID)
	if !got.Archived || !got.Enabled {
		t.Fatalf("archive must not touch Enabled: %+v", got)
	}
	if err := d.SetAutomationArchived(ctx, a.ID, false); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if list, _ := d.ListEnabledAutomations(ctx); len(list) != 1 {
		t.Fatal("restored automation must be listed again")
	}
	if err := d.SetAutomationArchived(ctx, "AUT-nope", true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown id: %v", err)
	}
	// Hook telemetry.
	_ = d.RecordHookFire(ctx, h.ID)
	_ = d.RecordHookFire(ctx, h.ID)
	hk, _ := d.GetHook(ctx, h.ID)
	if hk.FireCount != 2 || hk.LastFiredAt == 0 {
		t.Fatalf("hook telemetry = %+v, want 2 fires with a timestamp", hk)
	}
}
