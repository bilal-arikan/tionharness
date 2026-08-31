package agent

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func TestTerminalWorkerArchivedAfterNotificationPersisted(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	coord := newTestCoordinator(t, rt, 0)
	rt.coordRunFn = func(string) {}
	worker, err := rt.db.CreateSession(context.Background(), db.Session{
		Kind: "worker", Role: db.SessionRoleWorker, SourceID: "test:worker:archive", CoordinatorSessionID: coord,
	})
	if err != nil {
		t.Fatalf("create worker: %v", err)
	}

	if err := rt.notifyCoordinator(coord, "<task-notification>done</task-notification>", false, nil, worker.ID); err != nil {
		t.Fatalf("notify terminal worker: %v", err)
	}

	got, err := rt.db.GetSession(context.Background(), worker.ID)
	if err != nil {
		t.Fatalf("get worker: %v", err)
	}
	if got.State != "archived" {
		t.Fatalf("worker state = %q, want archived", got.State)
	}
	if note := lastCoordNote(t, rt, coord); note != "<task-notification>done</task-notification>" {
		t.Fatalf("persisted notification = %q", note)
	}
}

func TestTerminalWorkerNotArchivedWhenNotificationPersistFails(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	worker, err := rt.db.CreateSession(context.Background(), db.Session{
		Kind: "worker", Role: db.SessionRoleWorker, SourceID: "test:worker:persist-failure", CoordinatorSessionID: "missing-coordinator",
	})
	if err != nil {
		t.Fatalf("create worker: %v", err)
	}

	err = rt.notifyCoordinator("missing-coordinator", "<task-notification>done</task-notification>", false, nil, worker.ID)
	if err == nil {
		t.Fatal("expected notification persistence error")
	}

	got, err := rt.db.GetSession(context.Background(), worker.ID)
	if err != nil {
		t.Fatalf("get worker: %v", err)
	}
	if got.State == "archived" {
		t.Fatal("worker archived despite notification persistence failure")
	}
}
