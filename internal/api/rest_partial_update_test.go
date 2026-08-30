package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/skills"
)

func TestFlowPartialUpdatePreservesName(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	flow, err := wsp.DB.CreateFlow(context.Background(), db.Flow{Name: "kept", Graph: `{}`})
	if err != nil {
		t.Fatal(err)
	}
	rec := doJSON(t, s.Routes(), http.MethodPut, "/api/flows/"+flow.ID, map[string]any{"graph": map[string]any{}}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	got, _ := wsp.DB.GetFlow(context.Background(), flow.ID)
	if got.Name != "kept" {
		t.Fatalf("name = %q", got.Name)
	}
}

func TestAutomationPartialUpdatePreservesOptionalFields(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	agent, err := wsp.DB.CreateAgent(context.Background(), db.Agent{Name: "agent"})
	if err != nil {
		t.Fatal(err)
	}
	auto, err := wsp.DB.CreateAutomation(context.Background(), db.Automation{Name: "kept", TriggerKind: db.TriggerTag, TriggerTag: "done", TargetAgentID: agent.ID, PromptTemplate: "run", SpawnTags: []string{"one"}, SessionMode: "spawn", Enabled: true, MaxIterations: 3})
	if err != nil {
		t.Fatal(err)
	}
	rec := doJSON(t, s.Routes(), http.MethodPut, "/api/automations/"+auto.ID, map[string]any{"cooldownSec": 5}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	got, _ := wsp.DB.GetAutomation(context.Background(), auto.ID)
	if got.Name != "kept" || len(got.SpawnTags) != 1 || got.SpawnTags[0] != "one" || got.SessionMode != "spawn" {
		t.Fatalf("partial update lost fields: %+v", got)
	}
}

func TestSchedulePartialUpdatePreservesRecord(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	agent, err := wsp.DB.CreateAgent(context.Background(), db.Agent{Name: "agent"})
	if err != nil {
		t.Fatal(err)
	}
	schedule, err := wsp.DB.CreateSchedule(context.Background(), db.Schedule{Name: "kept", AgentID: agent.ID, CronExpr: "0 * * * *", Prompt: "kept prompt", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	rec := doJSON(t, s.Routes(), http.MethodPut, "/api/schedules/"+schedule.ID, map[string]any{"name": "changed"}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	got, _ := wsp.DB.GetSchedule(context.Background(), schedule.ID)
	if got.Name != "changed" || got.CronExpr != schedule.CronExpr || got.Prompt != schedule.Prompt || got.AgentID != agent.ID {
		t.Fatalf("partial update lost fields: %+v", got)
	}
}

func TestSkillPartialUpdatePreservesFields(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	store := wsp.Runtime.Skills()
	_, err := store.Create("partial-skill", skills.SkillInput{Name: "kept", Description: "description", WhenToUse: "when", Icon: "star", Color: "blue", Group: "group", Shared: true, Body: "body"})
	if err != nil {
		t.Fatal(err)
	}
	rec := doJSON(t, s.Routes(), http.MethodPut, "/api/skills/partial-skill", map[string]any{"description": "changed"}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	got, _ := store.Get("partial-skill")
	body, _ := store.Body("partial-skill")
	if got.Name != "kept" || got.Description != "changed" || got.WhenToUse != "when" || got.Icon != "star" || got.Color != "blue" || got.Group != "group" || !got.Shared || body != "body" {
		t.Fatalf("partial update lost fields: %+v body=%q", got, body)
	}
}

func TestTaskReferencesAreValidated(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	// Task creation normally launches detached board-automation and worktree
	// handlers. They are unrelated to reference validation and may still touch the
	// TempDir while Windows cleanup runs.
	wsp.DB.SetBoardHook(nil)
	task, err := wsp.DB.CreateTask(context.Background(), db.Task{Title: "task"})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, method, path string
		body               map[string]any
	}{
		{"create owner", http.MethodPost, "/api/tasks", map[string]any{"title": "new", "ownerAgentId": "missing"}},
		{"create flow", http.MethodPost, "/api/tasks", map[string]any{"title": "new", "flowId": "missing"}},
		{"update owner", http.MethodPut, "/api/tasks/" + task.ID, map[string]any{"ownerAgentId": "missing"}},
		{"update flow", http.MethodPut, "/api/tasks/" + task.ID, map[string]any{"flowId": "missing"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doJSON(t, s.Routes(), tt.method, tt.path, tt.body, nil)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestSettingsBridgeRejectsInvalidPatch(t *testing.T) {
	s, _ := newWorkspaceServer(t)
	before := s.settings.Get().Theme
	if _, err := s.SettingsBridge().Apply(`{"theme":"invalid"}`); err == nil {
		t.Fatal("expected validation error")
	}
	if got := s.settings.Get().Theme; got != before {
		t.Fatalf("theme changed from %q to %q", before, got)
	}
}
