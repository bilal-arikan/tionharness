package agent

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// TestSpawnWorkerOriginIsCoordinator: a worker's origin names the coordinator
// that spawned it and the tree root, without the spawner passing an origin.
func TestSpawnWorkerOriginIsCoordinator(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	seedSystemAgents(t, rt)
	base, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Coord", Provider: "anthropic", Model: "m"})
	if err != nil {
		t.Fatalf("create base agent: %v", err)
	}
	coord := newTestCoordinator(t, rt, 0)
	defer waitWorkersSettled(t, rt, coord)

	r1, err := rt.SpawnWorker(ctx, coord, "explore", "map the code", base.ID, WorkerSpec{})
	if err != nil {
		t.Fatalf("spawn worker: %v", err)
	}
	worker, err := rt.db.GetSession(ctx, r1.SessionID)
	if err != nil {
		t.Fatalf("get worker: %v", err)
	}
	o := worker.Lineage()
	if o.Kind != db.OriginCoordinator || o.TriggerSessionID != coord {
		t.Fatalf("worker origin = %+v, want coordinator←%s", o, coord)
	}
	if worker.RootSession() != coord {
		t.Fatalf("worker RootSession() = %q, want the root coordinator %q", worker.RootSession(), coord)
	}
	if o.At != worker.CreatedAt {
		t.Fatalf("origin.At = %d, want CreatedAt %d", o.At, worker.CreatedAt)
	}
}

// TestHandoffContinuationOrigin: a context-reset continuation is a handoff from
// the old session and stays in the old session's tree.
func TestHandoffContinuationOrigin(t *testing.T) {
	parent := db.Session{ID: "SES10", Kind: "chat", Title: "work",
		Origin: &db.SessionOrigin{Kind: db.OriginCoordinator, TriggerSessionID: "SES1", RootSessionID: "SES1"}}
	opts := handoffContinuationSpawnOpts(parent, HandoffOptions{})
	if opts.Origin == nil || opts.Origin.Kind != db.OriginHandoff {
		t.Fatalf("continuation origin = %+v, want handoff", opts.Origin)
	}
	if opts.Origin.TriggerSessionID != "SES10" {
		t.Fatalf("continuation trigger = %q, want the handed-off session SES10", opts.Origin.TriggerSessionID)
	}
	if opts.Origin.RootSessionID != "SES1" {
		t.Fatalf("continuation root = %q, want the parent's root SES1", opts.Origin.RootSessionID)
	}
	if opts.ParentSessionID != "SES10" {
		t.Fatalf("ParentSessionID must still mirror the handoff lineage, got %q", opts.ParentSessionID)
	}
}
