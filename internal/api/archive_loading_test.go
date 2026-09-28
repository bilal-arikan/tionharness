package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func TestArchivedTasksLoadOnlyOnRequest(t *testing.T) {
	h, wsp := taskRoutesFixture(t)
	ctx := context.Background()
	live, err := wsp.DB.CreateTask(ctx, db.Task{Title: "live"})
	if err != nil {
		t.Fatal(err)
	}
	old, err := wsp.DB.CreateTask(ctx, db.Task{Title: "old"})
	if err != nil {
		t.Fatal(err)
	}
	if err := wsp.DB.SetTaskArchived(ctx, old.ID, true); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		query string
		ids   []string
	}{
		{"", []string{live.ID}},
		{"?archived=only", []string{old.ID}},
		{"?archived=1", []string{live.ID, old.ID}},
	} {
		var rows []db.Task
		rec := doJSON(t, h, http.MethodGet, "/api/tasks"+tc.query, nil, &rows)
		if rec.Code != http.StatusOK || len(rows) != len(tc.ids) {
			t.Fatalf("%s: status=%d rows=%v", tc.query, rec.Code, rows)
		}
		for _, id := range tc.ids {
			found := false
			for _, row := range rows {
				if row.ID == id {
					found = true
				}
			}
			if !found {
				t.Fatalf("%s: missing %s", tc.query, id)
			}
		}
	}
}

func TestArchivedSessionsRequireExplicitRequest(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	ctx := context.Background()
	a, err := wsp.DB.CreateAgent(ctx, db.Agent{Name: "author"})
	if err != nil {
		t.Fatal(err)
	}
	var old db.Session
	rec := doJSON(t, s.Routes(), http.MethodPost, "/api/sessions", map[string]any{"agentId": a.ID, "title": "old"}, &old)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, s.Routes(), http.MethodPut, "/api/sessions/"+old.ID+"/state", map[string]any{"state": "archived"}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("archive: %d %s", rec.Code, rec.Body.String())
	}
	for _, tc := range []struct {
		query   string
		present bool
	}{
		{"", false}, {"?state=archived", true}, {"?state=all", true}, {"?ids=" + old.ID, true},
	} {
		var rows []db.Session
		rec = doJSON(t, s.Routes(), http.MethodGet, "/api/sessions"+tc.query, nil, &rows)
		if rec.Code != http.StatusOK {
			t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
		}
		found := false
		for _, row := range rows {
			if row.ID == old.ID {
				found = true
			}
		}
		if found != tc.present {
			t.Fatalf("%s: archived present=%v, want %v", tc.query, found, tc.present)
		}
	}
}
