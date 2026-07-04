package db

import (
	"context"
	"testing"
)

func TestAutomationLifecycle(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	a, err := d.CreateAutomation(ctx, Automation{
		Name:           "loop",
		TriggerTag:     " loop ", // trimmed on create
		TargetAgentID:  "AGT1",
		PromptTemplate: "go: {{result}}",
		Enabled:        true,
		MaxIterations:  3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.TriggerTag != "loop" {
		t.Fatalf("triggerTag not trimmed: %q", a.TriggerTag)
	}
	if a.ID == "" {
		t.Fatal("no id assigned")
	}

	// Only enabled ones surface to the engine.
	if got, _ := d.ListEnabledAutomations(ctx); len(got) != 1 {
		t.Fatalf("enabled count = %d, want 1", len(got))
	}

	// A fire increments the counter and records the spawned session.
	if err := d.RecordAutomationFire(ctx, a.ID, "SES9", ""); err != nil {
		t.Fatal(err)
	}
	got, _ := d.GetAutomation(ctx, a.ID)
	if got.IterationCount != 1 || got.LastSessionID != "SES9" {
		t.Fatalf("after fire: count=%d last=%q", got.IterationCount, got.LastSessionID)
	}

	// Disable then re-enable resets the counter (fresh loop budget).
	if err := d.SetAutomationEnabled(ctx, a.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := d.SetAutomationEnabled(ctx, a.ID, true); err != nil {
		t.Fatal(err)
	}
	got, _ = d.GetAutomation(ctx, a.ID)
	if got.IterationCount != 0 {
		t.Fatalf("re-enable did not reset counter: %d", got.IterationCount)
	}

	// ResetAutomationCount clears the counter without toggling.
	_ = d.RecordAutomationFire(ctx, a.ID, "SES10", "boom")
	if err := d.ResetAutomationCount(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	got, _ = d.GetAutomation(ctx, a.ID)
	if got.IterationCount != 0 || got.LastError != "" {
		t.Fatalf("reset left state: count=%d err=%q", got.IterationCount, got.LastError)
	}

	// Survives a reload from disk.
	d2, err := Open(d.root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d2.GetAutomation(ctx, a.ID); err != nil {
		t.Fatalf("automation not persisted: %v", err)
	}

	if err := d.DeleteAutomation(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.GetAutomation(ctx, a.ID); err == nil {
		t.Fatal("expected not-found after delete")
	}
}

func TestSetSessionTagsNormalizes(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	s, err := d.CreateSession(ctx, Session{AgentID: "AGT1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.SetSessionTags(ctx, s.ID, []string{" loop ", "loop", "", "x"}); err != nil {
		t.Fatal(err)
	}
	got, _ := d.GetSession(ctx, s.ID)
	if len(got.Tags) != 2 || got.Tags[0] != "loop" || got.Tags[1] != "x" {
		t.Fatalf("tags not normalized: %v", got.Tags)
	}
}

// TestAutomationFlowIDRoundTrip verifies a flow-backed automation persists its
// FlowID on create, keeps it through an update, and can be switched back to an
// agent target by clearing FlowID.
func TestAutomationFlowIDRoundTrip(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	a, err := d.CreateAutomation(ctx, Automation{
		TriggerTag:     "loop",
		FlowID:         "FLOW1",
		PromptTemplate: "go: {{result}}",
		Enabled:        true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.FlowID != "FLOW1" {
		t.Fatalf("create did not keep FlowID: %q", a.FlowID)
	}

	// Update to a different flow.
	a.FlowID = "FLOW2"
	if err := d.UpdateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}
	if got, _ := d.GetAutomation(ctx, a.ID); got.FlowID != "FLOW2" {
		t.Fatalf("update did not change FlowID: %q", got.FlowID)
	}

	// Switch to an agent target: clear FlowID, set TargetAgentID.
	a.FlowID = ""
	a.TargetAgentID = "AGT1"
	if err := d.UpdateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}
	got, _ := d.GetAutomation(ctx, a.ID)
	if got.FlowID != "" || got.TargetAgentID != "AGT1" {
		t.Fatalf("switch to agent failed: flow=%q agent=%q", got.FlowID, got.TargetAgentID)
	}
}

// TestScheduleFlowIDRoundTrip verifies a flow-backed schedule persists FlowID on
// create and keeps it through an update.
func TestScheduleFlowIDRoundTrip(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	sc, err := d.CreateSchedule(ctx, Schedule{
		FlowID:   "FLOW1",
		CronExpr: "0 9 * * *",
		Prompt:   "input",
		Enabled:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sc.FlowID != "FLOW1" {
		t.Fatalf("create did not keep FlowID: %q", sc.FlowID)
	}

	// Switch to an agent target: clear FlowID, set AgentID.
	sc.FlowID = ""
	sc.AgentID = "AGT1"
	if err := d.UpdateSchedule(ctx, sc); err != nil {
		t.Fatal(err)
	}
	got, _ := d.GetSchedule(ctx, sc.ID)
	if got.FlowID != "" || got.AgentID != "AGT1" {
		t.Fatalf("switch to agent failed: flow=%q agent=%q", got.FlowID, got.AgentID)
	}
}

func TestFlowEmojiRoundTrip(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	// Create with an emoji; it must persist.
	f, err := d.CreateFlow(ctx, Flow{Name: "Draft & review", Emoji: "📝"})
	if err != nil {
		t.Fatal(err)
	}
	if f.Emoji != "📝" {
		t.Fatalf("create did not keep Emoji: %q", f.Emoji)
	}

	// A name/graph save must NOT wipe the emoji (persisted independently).
	if err := d.UpdateFlow(ctx, Flow{ID: f.ID, Name: "Renamed", Graph: "{}"}); err != nil {
		t.Fatal(err)
	}
	got, _ := d.GetFlow(ctx, f.ID)
	if got.Emoji != "📝" {
		t.Fatalf("UpdateFlow wiped emoji: %q", got.Emoji)
	}
	if got.Name != "Renamed" {
		t.Fatalf("UpdateFlow did not apply name: %q", got.Name)
	}

	// SetFlowEmoji replaces the glyph; empty string clears it.
	if err := d.SetFlowEmoji(ctx, f.ID, "🚀"); err != nil {
		t.Fatal(err)
	}
	if got, _ = d.GetFlow(ctx, f.ID); got.Emoji != "🚀" {
		t.Fatalf("SetFlowEmoji did not apply: %q", got.Emoji)
	}
	if err := d.SetFlowEmoji(ctx, f.ID, ""); err != nil {
		t.Fatal(err)
	}
	if got, _ = d.GetFlow(ctx, f.ID); got.Emoji != "" {
		t.Fatalf("SetFlowEmoji did not clear: %q", got.Emoji)
	}
}
