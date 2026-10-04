package db

import (
	"context"
	"os"
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

// TestUpdateAutomationPersistsAllFields is the regression guard for the
// field-by-field UpdateAutomation copy that silently dropped fields it forgot to
// list: the counter fields and sessionMode round-tripped through create
// but were lost on update. UpdateAutomation now replaces the whole configuration,
// so this test fails loudly if any config field stops persisting on update. It also
// checks that the runtime bookkeeping (iteration count) is preserved across an edit.
func TestUpdateAutomationPersistsAllFields(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	a, err := d.CreateAutomation(ctx, Automation{
		TriggerKind:    TriggerBoard,
		BoardToState:   "review",
		BoardPriority:  1,
		SessionMode:    SessionModeContinue,
		TargetAgentID:  "AGT1",
		PromptTemplate: "go",
		Enabled:        true,
		MaxIterations:  3,
	})
	if err != nil {
		t.Fatal(err)
	}
	// A prior fire leaves bookkeeping the edit must preserve.
	if err := d.RecordAutomationFire(ctx, a.ID, "SES1", ""); err != nil {
		t.Fatal(err)
	}

	// Edit every config field the update paths can touch.
	a.BoardToState = "done"
	a.BoardPriority = 5
	a.SessionMode = SessionModeSpawn
	a.MaxIterations = 7
	a.PromptTemplate = "changed"
	if err := d.UpdateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}

	got, _ := d.GetAutomation(ctx, a.ID)
	switch {
	case got.BoardToState != "done":
		t.Errorf("boardToState not persisted: %q", got.BoardToState)
	case got.BoardPriority != 5:
		t.Errorf("boardPriority not persisted: %d", got.BoardPriority)
	case got.SessionMode != SessionModeSpawn:
		t.Errorf("sessionMode not persisted: %q", got.SessionMode)
	case got.MaxIterations != 7:
		t.Errorf("maxIterations not persisted: %d", got.MaxIterations)
	case got.PromptTemplate != "changed":
		t.Errorf("promptTemplate not persisted: %q", got.PromptTemplate)
	}
	// Bookkeeping survives the config edit.
	if got.IterationCount != 1 || got.LastSessionID != "SES1" {
		t.Errorf("update clobbered bookkeeping: count=%d last=%q", got.IterationCount, got.LastSessionID)
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

// TestRetiredTokenAutomationSkippedOnLoad pins the persistence contract for the
// removed "token" trigger kind: a rule file written before the removal stays on
// disk untouched but is never loaded, so it cannot surface in the UI, the API or
// the engine.
func TestRetiredTokenAutomationSkippedOnLoad(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	live, err := d.CreateAutomation(ctx, Automation{
		TriggerKind: TriggerTag, TriggerTag: "loop", TargetAgentID: "AGT1",
		PromptTemplate: "go", Enabled: true, MaxIterations: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	legacy := d.dir(dirAutomations, "AUT_LEGACY_TOKEN.json")
	raw := `{"id":"AUT_LEGACY_TOKEN","name":"old","triggerKind":"token","tokenScope":"workspace","tokenThreshold":150000,"targetAgentId":"AGT1","promptTemplate":"go","enabled":true,"maxIterations":5}`
	if err := os.WriteFile(legacy, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	d2, err := Open(d.root)
	if err != nil {
		t.Fatal(err)
	}
	all, _ := d2.ListAutomations(ctx)
	if len(all) != 1 || all[0].ID != live.ID {
		t.Fatalf("retired token rule surfaced on load: %+v", all)
	}
	if _, err := d2.GetAutomation(ctx, "AUT_LEGACY_TOKEN"); err == nil {
		t.Fatal("retired token rule must not be loadable by id")
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Fatalf("retired rule file must stay on disk untouched: %v", err)
	}
}
