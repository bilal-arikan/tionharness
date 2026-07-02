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
