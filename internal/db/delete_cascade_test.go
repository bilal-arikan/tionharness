package db

import (
	"context"
	"testing"
)

// TestDeleteAgentCascadesSchedulesAndTasks verifies that deleting an agent also
// removes the schedules bound to it and the tasks it owns (with their runs),
// while leaving records belonging to a different agent untouched.
func TestDeleteAgentCascadesSchedulesAndTasks(t *testing.T) {
	ctx := context.Background()
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	victim, err := d.CreateAgent(ctx, Agent{Name: "Victim"})
	if err != nil {
		t.Fatalf("seed victim: %v", err)
	}
	keep, err := d.CreateAgent(ctx, Agent{Name: "Keep"})
	if err != nil {
		t.Fatalf("seed keep: %v", err)
	}

	victimSched, err := d.CreateSchedule(ctx, Schedule{AgentID: victim.ID, CronExpr: "* * * * *", Prompt: "x"})
	if err != nil {
		t.Fatalf("seed victim schedule: %v", err)
	}
	keepSched, err := d.CreateSchedule(ctx, Schedule{AgentID: keep.ID, CronExpr: "* * * * *", Prompt: "y"})
	if err != nil {
		t.Fatalf("seed keep schedule: %v", err)
	}

	victimTask, err := d.CreateTask(ctx, Task{Title: "T", OwnerAgentID: victim.ID})
	if err != nil {
		t.Fatalf("seed victim task: %v", err)
	}
	keepTask, err := d.CreateTask(ctx, Task{Title: "K", OwnerAgentID: keep.ID})
	if err != nil {
		t.Fatalf("seed keep task: %v", err)
	}
	// Runs are written by execution layers, not a public store method; seed the
	// in-memory map directly (same package) to exercise the cascade.
	d.mu.Lock()
	d.runs["run-victim"] = Run{ID: "run-victim", TaskID: victimTask.ID, AgentID: victim.ID}
	d.mu.Unlock()

	if err := d.DeleteAgent(ctx, victim.ID); err != nil {
		t.Fatalf("delete agent: %v", err)
	}

	if _, err := d.GetSchedule(ctx, victimSched.ID); err == nil {
		t.Error("victim schedule should be deleted")
	}
	if _, err := d.GetSchedule(ctx, keepSched.ID); err != nil {
		t.Errorf("keep schedule should survive: %v", err)
	}
	if _, err := d.GetTask(ctx, victimTask.ID); err == nil {
		t.Error("victim task should be deleted")
	}
	if _, err := d.GetTask(ctx, keepTask.ID); err != nil {
		t.Errorf("keep task should survive: %v", err)
	}
	d.mu.RLock()
	_, runExists := d.runs["run-victim"]
	d.mu.RUnlock()
	if runExists {
		t.Error("victim run should be deleted")
	}
}

// TestRemoveSkillFromAgents verifies the slug is stripped from every agent that
// referenced it and that the change is persisted, while unrelated slugs stay.
func TestRemoveSkillFromAgents(t *testing.T) {
	ctx := context.Background()
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	a, err := d.CreateAgent(ctx, Agent{Name: "A", Skills: []string{"alpha", "beta"}})
	if err != nil {
		t.Fatalf("seed a: %v", err)
	}
	b, err := d.CreateAgent(ctx, Agent{Name: "B", Skills: []string{"gamma"}})
	if err != nil {
		t.Fatalf("seed b: %v", err)
	}

	n, err := d.RemoveSkillFromAgents(ctx, "beta")
	if err != nil {
		t.Fatalf("remove skill: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 agent updated, got %d", n)
	}

	got, err := d.GetAgent(ctx, a.ID)
	if err != nil {
		t.Fatalf("get a: %v", err)
	}
	if len(got.Skills) != 1 || got.Skills[0] != "alpha" {
		t.Errorf("agent A skills = %v, want [alpha]", got.Skills)
	}
	gotB, err := d.GetAgent(ctx, b.ID)
	if err != nil {
		t.Fatalf("get b: %v", err)
	}
	if len(gotB.Skills) != 1 || gotB.Skills[0] != "gamma" {
		t.Errorf("agent B skills = %v, want [gamma]", gotB.Skills)
	}
}
