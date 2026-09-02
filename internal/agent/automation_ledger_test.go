package agent

import (
	"context"
	"log/slog"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/events"
)

// TestGuardSkipIsRecordedAndAnnounced: a guard skip lands in the rule's fire
// ledger with its reason and on the workspace stream as automation_fire/skipped;
// an archived rule is skipped for that reason.
func TestGuardSkipIsRecordedAndAnnounced(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	rt.wsID = "WS-test"
	drain := collectWS(t, rt)
	ctx := context.Background()
	ag, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "A", Provider: "anthropic", Model: "m"})
	e := NewAutomationEngine(rt.db, rt, slog.New(slog.NewTextHandler(discardWriter{}, nil)))

	cooled, _ := rt.db.CreateAutomation(ctx, db.Automation{Name: "cool", TriggerTag: "t", PromptTemplate: "p", TargetAgentID: ag.ID, MaxIterations: 5, CooldownSec: 3600, Enabled: true})
	if err := rt.db.RecordAutomationFire(ctx, cooled.ID, "SES1", ""); err != nil {
		t.Fatalf("seed last fire: %v", err)
	}
	cooled, _ = rt.db.GetAutomation(ctx, cooled.ID)
	if e.guardsPass(ctx, cooled) {
		t.Fatal("cooldown must block")
	}
	archived := cooled
	archived.Archived = true
	archived.CooldownSec = 0
	if e.guardsPass(ctx, archived) {
		t.Fatal("archived must block")
	}

	recs, err := rt.db.ListAutomationFires(ctx, cooled.ID, 0)
	if err != nil || len(recs) != 2 {
		t.Fatalf("ledger = %+v err=%v, want 2 skips", recs, err)
	}
	if recs[0].Reason != db.AutomationSkipArchived || recs[1].Reason != db.AutomationSkipCooldown || recs[1].Outcome != db.AutomationFireSkipped {
		t.Fatalf("ledger reasons = %q / %q, want archived then cooldown", recs[0].Reason, recs[1].Reason)
	}
	var fires []AutomationFirePayload
	for _, ev := range drain() {
		if ev.Type == events.TypeWSAutomationFire {
			fires = append(fires, decodeData[AutomationFirePayload](t, ev))
		}
	}
	if len(fires) != 2 || fires[0].Outcome != db.AutomationFireSkipped || fires[0].Reason != db.AutomationSkipCooldown || fires[1].Reason != db.AutomationSkipArchived {
		t.Fatalf("workspace events = %+v, want two skipped fires (cooldown, archived)", fires)
	}
}
