package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

func taskRoutesFixture(t *testing.T) (http.Handler, *workspace.Workspace) {
	t.Helper()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	wsp := &workspace.Workspace{DB: database}
	mux := http.NewServeMux()
	newTestServer().registerTaskRoutes(mux)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), workspaceCtxKey, wsp)
		mux.ServeHTTP(w, r.WithContext(ctx))
	}), wsp
}

func TestTaskUnknownSubpathsReturnNotFound(t *testing.T) {
	handler, wsp := taskRoutesFixture(t)
	task, err := wsp.DB.CreateTask(context.Background(), db.Task{Title: "task"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	for _, subpath := range []string{"bogus-subpath"} {
		t.Run(subpath, func(t *testing.T) {
			rec := doJSON(t, handler, http.MethodPost, "/api/tasks/"+task.ID+"/"+subpath, nil, nil)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusNotFound, rec.Body.String())
			}
			if got := rec.Body.String(); got != "{\"error\":\"task subpath not found\"}\n" {
				t.Fatalf("body = %q", got)
			}
		})
	}
}

func TestTaskUnarchiveRouteRestoresArchivedTask(t *testing.T) {
	handler, wsp := taskRoutesFixture(t)
	task, err := wsp.DB.CreateTask(context.Background(), db.Task{Title: "task", BoardState: db.BoardDone})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := wsp.DB.SetTaskArchived(context.Background(), task.ID, true); err != nil {
		t.Fatalf("archive task: %v", err)
	}

	rec := doJSON(t, handler, http.MethodPost, "/api/tasks/"+task.ID+"/unarchive", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	got, err := wsp.DB.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.Archived {
		t.Fatal("task remained archived after successful unarchive response")
	}
}

func TestTaskUnarchiveRouteRejectsMissingTask(t *testing.T) {
	handler, _ := taskRoutesFixture(t)
	rec := doJSON(t, handler, http.MethodPost, "/api/tasks/TSK404/unarchive", nil, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestTaskArchiveRouteTogglesArchivedState(t *testing.T) {
	handler, wsp := taskRoutesFixture(t)
	task, err := wsp.DB.CreateTask(context.Background(), db.Task{Title: "task"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	for _, archived := range []bool{true, false} {
		t.Run(map[bool]string{true: "archive", false: "unarchive"}[archived], func(t *testing.T) {
			rec := doJSON(t, handler, http.MethodPost, "/api/tasks/"+task.ID+"/archive", map[string]any{"archived": archived}, nil)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
			}
			got, err := wsp.DB.GetTask(context.Background(), task.ID)
			if err != nil {
				t.Fatalf("get task: %v", err)
			}
			if got.Archived != archived {
				t.Fatalf("archived = %v, want %v", got.Archived, archived)
			}
		})
	}
}
