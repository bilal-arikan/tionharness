package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/archive"
	"github.com/bilal-arikan/tionharness/internal/db"
)

// TestSelfManagementToolsRefuseArchivedAgentTargets: create/update of a
// schedule, automation or task owner pointing at an archived agent returns the
// archive error; an update that keeps the already-stored archived target (and
// changes something else) still succeeds.
func TestSelfManagementToolsRefuseArchivedAgentTargets(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t)
	noReload := func(context.Context) error { return nil }
	live, err := database.CreateAgent(ctx, db.Agent{Name: "live"})
	if err != nil {
		t.Fatal(err)
	}
	old, err := database.CreateAgent(ctx, db.Agent{Name: "old"})
	if err != nil {
		t.Fatal(err)
	}
	oldSched, err := database.CreateSchedule(ctx, db.Schedule{AgentID: old.ID, CronExpr: "0 * * * *", Prompt: "p"})
	if err != nil {
		t.Fatal(err)
	}
	liveSched, err := database.CreateSchedule(ctx, db.Schedule{AgentID: live.ID, CronExpr: "0 * * * *", Prompt: "p"})
	if err != nil {
		t.Fatal(err)
	}
	oldAuto, err := database.CreateAutomation(ctx, db.Automation{TriggerKind: db.TriggerTag, TriggerTag: "done", TargetAgentID: old.ID, PromptTemplate: "run", Enabled: true, MaxIterations: 3})
	if err != nil {
		t.Fatal(err)
	}
	liveAuto, err := database.CreateAutomation(ctx, db.Automation{TriggerKind: db.TriggerTag, TriggerTag: "done", TargetAgentID: live.ID, PromptTemplate: "run", Enabled: true, MaxIterations: 3})
	if err != nil {
		t.Fatal(err)
	}
	oldTask, err := database.CreateTask(ctx, db.Task{Title: "t", OwnerAgentID: old.ID})
	if err != nil {
		t.Fatal(err)
	}
	liveTask, err := database.CreateTask(ctx, db.Task{Title: "lt", OwnerAgentID: live.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.SetAgentArchived(ctx, old.ID, true); err != nil {
		t.Fatal(err)
	}

	type caller interface {
		Call(context.Context, json.RawMessage) (string, error)
	}
	createSched := NewCreateScheduleTool(database, "actor", noReload)
	updateSched := NewUpdateScheduleTool(database, "actor", noReload)
	createAuto := NewCreateAutomationTool(database, "actor")
	updateAuto := NewUpdateAutomationTool(database, "actor")
	createTask := NewCreateTaskTool(database, "actor")
	updateTask := NewUpdateTaskTool(database, "actor")

	refused := []struct {
		name  string
		tool  caller
		input string
	}{
		{"create_schedule", createSched, `{"agentId":"` + old.ID + `","cronExpr":"0 * * * *","prompt":"p"}`},
		{"update_schedule", updateSched, `{"id":"` + liveSched.ID + `","agentId":"` + old.ID + `"}`},
		{"create_automation", createAuto, `{"triggerKind":"tag","triggerTag":"done","targetAgentId":"` + old.ID + `","promptTemplate":"run"}`},
		{"update_automation", updateAuto, `{"id":"` + liveAuto.ID + `","targetAgentId":"` + old.ID + `"}`},
		{"create_task", createTask, `{"title":"x","ownerAgentId":"` + old.ID + `"}`},
		{"update_task", updateTask, `{"id":"` + liveTask.ID + `","ownerAgentId":"` + old.ID + `"}`},
	}
	for _, c := range refused {
		if _, err := c.tool.Call(ctx, json.RawMessage(c.input)); !errors.Is(err, archive.ErrArchived) {
			t.Errorf("%s: err = %v, want archive.ErrArchived", c.name, err)
		}
	}

	kept := []struct {
		name  string
		tool  caller
		input string
	}{
		{"update_schedule", updateSched, `{"id":"` + oldSched.ID + `","agentId":"` + old.ID + `","name":"renamed"}`},
		{"update_automation", updateAuto, `{"id":"` + oldAuto.ID + `","targetAgentId":"` + old.ID + `","name":"renamed"}`},
		{"update_task", updateTask, `{"id":"` + oldTask.ID + `","ownerAgentId":"` + old.ID + `","title":"renamed"}`},
	}
	for _, c := range kept {
		if _, err := c.tool.Call(ctx, json.RawMessage(c.input)); err != nil {
			t.Errorf("%s keeping stored archived target: %v", c.name, err)
		}
	}
}
