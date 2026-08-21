package agent

import (
	"context"
	"path/filepath"
	"testing"
)

// TestSpawnWorkerInheritsCoordinatorCwd locks the fix for workers that used to
// land in the WORKSPACE default directory: a coordinator working in repo A fans
// out workers that must also run in repo A. Before this, the repo path written
// into the task text was mere prose while the worker silently ran elsewhere and
// every build failed with "does not contain main module".
func TestSpawnWorkerInheritsCoordinatorCwd(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	rt.SetDefaultWorkDir(filepath.Join(t.TempDir(), "workspace-default"))
	ctx := context.Background()

	coordDir := filepath.Join(t.TempDir(), "repo-a")
	coord := newTestCoordinator(t, rt, 0)
	if err := rt.db.SetSessionWorkingDir(ctx, coord, coordDir); err != nil {
		t.Fatalf("set coordinator cwd: %v", err)
	}

	res, err := rt.SpawnWorker(ctx, coord, "Coord0", "do the thing", "", WorkerSpec{})
	if err != nil {
		t.Fatalf("spawn worker: %v", err)
	}
	sess, err := rt.db.GetSession(ctx, res.SessionID)
	if err != nil {
		t.Fatalf("get worker session: %v", err)
	}
	if sess.WorkingDir != coordDir {
		t.Errorf("worker cwd = %q, want the coordinator's %q", sess.WorkingDir, coordDir)
	}
}

// TestSpawnWorkerExplicitCwdWins verifies the spec value overrides the inherited
// directory, which is how a coordinator sends a worker into a different repo.
func TestSpawnWorkerExplicitCwdWins(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()

	coordDir := filepath.Join(t.TempDir(), "repo-a")
	targetDir := filepath.Join(t.TempDir(), "repo-b")
	coord := newTestCoordinator(t, rt, 0)
	if err := rt.db.SetSessionWorkingDir(ctx, coord, coordDir); err != nil {
		t.Fatalf("set coordinator cwd: %v", err)
	}

	res, err := rt.SpawnWorker(ctx, coord, "Coord0", "do the thing", "", WorkerSpec{WorkingDir: targetDir})
	if err != nil {
		t.Fatalf("spawn worker: %v", err)
	}
	sess, err := rt.db.GetSession(ctx, res.SessionID)
	if err != nil {
		t.Fatalf("get worker session: %v", err)
	}
	if sess.WorkingDir != targetDir {
		t.Errorf("worker cwd = %q, want the requested %q", sess.WorkingDir, targetDir)
	}
}
