package api

import (
	"net/http"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// TestListSchedulesOneShotVisibility pins the routine-list filter: a PENDING wake
// (never delivered) stays hidden, but a SPENT one — a wake whose delivery was
// attempted and failed — is listed, because that row is the only handle the user
// has for deleting it.
func TestListSchedulesOneShotVisibility(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	agent, err := wsp.DB.CreateAgent(t.Context(), db.Agent{Name: "agent"})
	if err != nil {
		t.Fatal(err)
	}
	wake, err := wsp.DB.CreateSchedule(t.Context(), db.Schedule{
		AgentID: agent.ID,
		Prompt:  "continue",
		Enabled: true,
		OneShot: true,
		FireAt:  1700000000,
	})
	if err != nil {
		t.Fatal(err)
	}

	var listed []db.Schedule
	rec := doJSON(t, s.Routes(), http.MethodGet, "/api/schedules", nil, &listed)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	for _, sc := range listed {
		if sc.ID == wake.ID {
			t.Fatalf("pending one-shot wake %s must stay hidden from the routine list", wake.ID)
		}
	}

	if err := wsp.DB.SetScheduleDelivery(t.Context(), wake.ID, "failure", "session gone", 0); err != nil {
		t.Fatal(err)
	}
	listed = nil
	rec = doJSON(t, s.Routes(), http.MethodGet, "/api/schedules", nil, &listed)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	found := false
	for _, sc := range listed {
		if sc.ID == wake.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("spent one-shot wake %s must be listed so it can be deleted", wake.ID)
	}
}

// TestUpdateScheduleRejectsOneShot: a wake has no cron and the scheduler owns its
// fire time, so editing it must fail loudly instead of writing a half-valid row.
func TestUpdateScheduleRejectsOneShot(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	agent, err := wsp.DB.CreateAgent(t.Context(), db.Agent{Name: "agent"})
	if err != nil {
		t.Fatal(err)
	}
	wake, err := wsp.DB.CreateSchedule(t.Context(), db.Schedule{
		AgentID: agent.ID,
		Prompt:  "continue",
		OneShot: true,
		FireAt:  1700000000,
	})
	if err != nil {
		t.Fatal(err)
	}
	rec := doJSON(t, s.Routes(), http.MethodPut, "/api/schedules/"+wake.ID, map[string]any{
		"cronExpr": "0 * * * *",
	}, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("update one-shot = %d %s, want 400", rec.Code, rec.Body.String())
	}
}
