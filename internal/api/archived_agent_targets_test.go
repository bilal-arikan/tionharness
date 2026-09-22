package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// TestArchivedAgentTargetsRefused covers the write-time archive gate: a
// schedule, automation or task owner cannot be pointed at an archived agent
// (409 with the archive error), while an edit that keeps the already-stored
// archived target is still accepted so the record stays editable.
func TestArchivedAgentTargetsRefused(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	h := s.Routes()
	ctx := t.Context()
	live, err := wsp.DB.CreateAgent(ctx, db.Agent{Name: "live"})
	if err != nil {
		t.Fatal(err)
	}
	old, err := wsp.DB.CreateAgent(ctx, db.Agent{Name: "old"})
	if err != nil {
		t.Fatal(err)
	}

	// Records created while "old" was still live, then the agent is archived.
	sched, err := wsp.DB.CreateSchedule(ctx, db.Schedule{Name: "s", AgentID: old.ID, CronExpr: "0 * * * *", Prompt: "p"})
	if err != nil {
		t.Fatal(err)
	}
	auto, err := wsp.DB.CreateAutomation(ctx, db.Automation{Name: "a", TriggerKind: db.TriggerTag, TriggerTag: "done", TargetAgentID: old.ID, PromptTemplate: "run", Enabled: true, MaxIterations: 3})
	if err != nil {
		t.Fatal(err)
	}
	task, err := wsp.DB.CreateTask(ctx, db.Task{Title: "t", OwnerAgentID: old.ID})
	if err != nil {
		t.Fatal(err)
	}
	liveSched, err := wsp.DB.CreateSchedule(ctx, db.Schedule{Name: "ls", AgentID: live.ID, CronExpr: "0 * * * *", Prompt: "p"})
	if err != nil {
		t.Fatal(err)
	}
	liveAuto, err := wsp.DB.CreateAutomation(ctx, db.Automation{Name: "la", TriggerKind: db.TriggerTag, TriggerTag: "done", TargetAgentID: live.ID, PromptTemplate: "run", Enabled: true, MaxIterations: 3})
	if err != nil {
		t.Fatal(err)
	}
	liveTask, err := wsp.DB.CreateTask(ctx, db.Task{Title: "lt", OwnerAgentID: live.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wsp.DB.SetAgentArchived(ctx, old.ID, true); err != nil {
		t.Fatal(err)
	}

	refused := []struct {
		name, method, path string
		body               map[string]any
	}{
		{"create schedule", http.MethodPost, "/api/schedules",
			map[string]any{"agentId": old.ID, "cronExpr": "0 * * * *", "prompt": "p"}},
		{"retarget schedule", http.MethodPut, "/api/schedules/" + liveSched.ID,
			map[string]any{"agentId": old.ID}},
		{"create automation", http.MethodPost, "/api/automations",
			map[string]any{"triggerKind": db.TriggerTag, "triggerTag": "done", "targetAgentId": old.ID, "promptTemplate": "run"}},
		{"retarget automation", http.MethodPut, "/api/automations/" + liveAuto.ID,
			map[string]any{"targetAgentId": old.ID}},
		{"create task", http.MethodPost, "/api/tasks",
			map[string]any{"title": "x", "ownerAgentId": old.ID}},
		{"reassign task", http.MethodPut, "/api/tasks/" + liveTask.ID,
			map[string]any{"ownerAgentId": old.ID}},
	}
	for _, c := range refused {
		rec := doJSON(t, h, c.method, c.path, c.body, nil)
		if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "archived") {
			t.Errorf("%s: got %d %s, want 409 archived", c.name, rec.Code, rec.Body.String())
		}
	}

	// Keeping the stored (archived) target while editing something else is allowed.
	kept := []struct {
		name, path string
		body       map[string]any
	}{
		{"rename schedule", "/api/schedules/" + sched.ID, map[string]any{"agentId": old.ID, "name": "renamed"}},
		{"rename automation", "/api/automations/" + auto.ID, map[string]any{"targetAgentId": old.ID, "name": "renamed"}},
		{"rename task", "/api/tasks/" + task.ID, map[string]any{"ownerAgentId": old.ID, "title": "renamed"}},
	}
	for _, c := range kept {
		rec := doJSON(t, h, http.MethodPut, c.path, c.body, nil)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: got %d %s, want 200", c.name, rec.Code, rec.Body.String())
		}
	}
}
