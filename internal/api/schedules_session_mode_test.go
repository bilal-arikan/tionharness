package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// TestScheduleSessionMode covers the create/update contract of the schedule
// sessionMode field: a known mode is persisted, an unknown one is rejected with
// 400 instead of being silently stored (which would resolve to reuse at fire time
// and hide the typo).
func TestScheduleSessionMode(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	agent, err := wsp.DB.CreateAgent(t.Context(), db.Agent{Name: "agent"})
	if err != nil {
		t.Fatal(err)
	}
	body := map[string]any{
		"agentId":     agent.ID,
		"cronExpr":    "0 * * * *",
		"prompt":      "daily report",
		"sessionMode": db.ScheduleSessionModeSpawn,
	}
	var created db.Schedule
	rec := doJSON(t, s.Routes(), http.MethodPost, "/api/schedules", body, &created)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	if created.SessionMode != db.ScheduleSessionModeSpawn {
		t.Fatalf("created sessionMode = %q, want %q", created.SessionMode, db.ScheduleSessionModeSpawn)
	}

	body["sessionMode"] = "bogus"
	rec = doJSON(t, s.Routes(), http.MethodPost, "/api/schedules", body, nil)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid sessionMode") {
		t.Fatalf("create with bogus mode = %d %s, want 400 invalid sessionMode", rec.Code, rec.Body.String())
	}

	var updated db.Schedule
	rec = doJSON(t, s.Routes(), http.MethodPut, "/api/schedules/"+created.ID, map[string]any{
		"cronExpr":    "0 * * * *",
		"sessionMode": db.ScheduleSessionModeReuse,
	}, &updated)
	if rec.Code != http.StatusOK {
		t.Fatalf("update status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if updated.SessionMode != db.ScheduleSessionModeReuse {
		t.Fatalf("updated sessionMode = %q, want %q", updated.SessionMode, db.ScheduleSessionModeReuse)
	}

	rec = doJSON(t, s.Routes(), http.MethodPut, "/api/schedules/"+created.ID, map[string]any{
		"cronExpr":    "0 * * * *",
		"sessionMode": "bogus",
	}, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("update with bogus mode = %d %s, want 400", rec.Code, rec.Body.String())
	}
}
