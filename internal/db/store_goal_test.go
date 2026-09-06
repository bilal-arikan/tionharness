package db

import (
	"context"
	"testing"
)

func fp(v float64) *float64 { return &v }

// TestGoalStoreLifecycle: create → edit (revision appended, raw text kept) →
// status change → reload from disk → delete.
func TestGoalStoreLifecycle(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	d, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	g, err := d.CreateGoal(ctx, Goal{
		Name: "Ucuz inceleme", RawText: "incelemeler pahalı",
		Primary:    GoalMetric{Metric: "recipe.avgCostUSD", Direction: "min"},
		Guardrails: []GoalGuardrail{{Metric: "recipe.successRate", Min: fp(0.9)}},
	}, GoalByWriter, "written")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if g.ID != "GOL1" || g.Status != GoalStatusDraft || g.Policy.Mode != GoalModePropose || g.CreatedBy != GoalByWriter {
		t.Fatalf("defaults: %+v", g)
	}
	if len(g.History) != 1 || g.History[0].By != GoalByWriter || g.History[0].Fields[0] != "created" {
		t.Fatalf("first revision: %+v", g.History)
	}

	// User edit: name + guardrail change → one revision naming both fields;
	// RawText and CreatedBy survive whatever the caller sends.
	edit := g
	edit.Name = "Kod incelemesi ucuzlasın"
	edit.Guardrails = []GoalGuardrail{{Metric: "recipe.successRate", Min: fp(0.95)}}
	edit.RawText = "tampered"
	edit.CreatedBy = "someone"
	edit.History = nil
	updated, err := d.UpdateGoal(ctx, edit, GoalByUser, "")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.RawText != "incelemeler pahalı" || updated.CreatedBy != GoalByWriter {
		t.Fatalf("update must keep raw text and provenance: %+v", updated)
	}
	if len(updated.History) != 2 {
		t.Fatalf("history len = %d, want 2", len(updated.History))
	}
	rev := updated.History[1]
	if rev.By != GoalByUser || len(rev.Fields) != 2 || rev.Fields[0] != "name" || rev.Fields[1] != "guardrails" {
		t.Fatalf("revision = %+v", rev)
	}
	// No-op edit appends nothing.
	same, _ := d.UpdateGoal(ctx, updated, GoalByUser, "")
	if len(same.History) != 2 {
		t.Fatalf("no-op edit must not append a revision, got %d", len(same.History))
	}

	// Status change is its own revision; repeating it is a no-op.
	act, err := d.SetGoalStatus(ctx, g.ID, GoalStatusActive, GoalByUser)
	if err != nil || act.Status != GoalStatusActive || len(act.History) != 3 || act.History[2].Note != "draft → active" {
		t.Fatalf("status: %v %+v", err, act)
	}
	if again, _ := d.SetGoalStatus(ctx, g.ID, GoalStatusActive, GoalByUser); len(again.History) != 3 {
		t.Fatal("same-status set must not append")
	}
	if err := d.ReplaceGoalRawText(ctx, g.ID, "yeni cümle"); err != nil {
		t.Fatalf("raw text: %v", err)
	}

	// Ordering: active first, then drafts by priority, then newest.
	d2, _ := d.CreateGoal(ctx, Goal{Name: "B", Priority: 1, Primary: GoalMetric{Metric: "board.cycleTimeSec", Direction: "min"}}, GoalByWriter, "")
	d3, _ := d.CreateGoal(ctx, Goal{Name: "C", Priority: 4, Primary: GoalMetric{Metric: "board.cycleTimeSec", Direction: "min"}}, GoalByWriter, "")
	list, _ := d.ListGoals(ctx)
	if len(list) != 3 || list[0].ID != g.ID || list[1].ID != d2.ID || list[2].ID != d3.ID {
		t.Fatalf("order = %v", []string{list[0].ID, list[1].ID, list[2].ID})
	}
	drafts, _ := d.ListGoalsByStatus(ctx, GoalStatusDraft)
	if len(drafts) != 2 {
		t.Fatalf("drafts = %d", len(drafts))
	}
	_ = d.Close()

	// Reload from disk.
	d, err = Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer d.Close()
	got, err := d.GetGoal(ctx, g.ID)
	if err != nil {
		t.Fatalf("get after reload: %v", err)
	}
	if got.Name != "Kod incelemesi ucuzlasın" || got.RawText != "yeni cümle" || got.Status != GoalStatusActive || len(got.History) != 3 {
		t.Fatalf("reloaded = %+v", got)
	}
	if got.Guardrails[0].Min == nil || *got.Guardrails[0].Min != 0.95 {
		t.Fatalf("guardrail bound lost: %+v", got.Guardrails)
	}
	if err := d.DeleteGoal(ctx, g.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := d.GetGoal(ctx, g.ID); err != ErrNotFound {
		t.Fatalf("after delete: %v", err)
	}
	if err := d.DeleteGoal(ctx, g.ID); err != ErrNotFound {
		t.Fatalf("double delete: %v", err)
	}
}
