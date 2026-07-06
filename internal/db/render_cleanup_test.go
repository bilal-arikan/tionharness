package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeRenderFile drops a file into a session's render dir and back-dates its
// modtime by age (0 = leave current).
func writeRenderFile(t *testing.T, d *DB, sessionID, name string, age time.Duration) string {
	t.Helper()
	dir := d.RenderDir(sessionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir render: %v", err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("<h1>x</h1>"), 0o644); err != nil {
		t.Fatalf("write render file: %v", err)
	}
	if age > 0 {
		old := time.Now().Add(-age)
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatalf("chtimes: %v", err)
		}
	}
	return p
}

// TestDeleteSessionRemovesRenderDir verifies session deletion also drops its
// transient render_template output.
func TestDeleteSessionRemovesRenderDir(t *testing.T) {
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	agent, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	sess, _ := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "T"})
	writeRenderFile(t, d, sess.ID, "report-ab12.html", 0)
	dir := d.RenderDir(sess.ID)

	if err := d.DeleteSession(ctx, sess.ID); err != nil {
		t.Fatalf("delete session: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("render dir should be removed with the session, stat err = %v", err)
	}
}

// TestCleanupRendersDropsOrphanDirs verifies the startup sweep removes render
// dirs whose session no longer exists but keeps live-session dirs.
func TestCleanupRendersDropsOrphanDirs(t *testing.T) {
	ctx := context.Background()
	storeDir := filepath.Join(t.TempDir(), "store")
	d, err := Open(storeDir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	agent, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	live, _ := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "live"})

	writeRenderFile(t, d, live.ID, "keep.html", 0) // live session, fresh
	orphanDir := d.RenderDir("ghost-session-id")   // no such session
	if err := os.MkdirAll(orphanDir, 0o755); err != nil {
		t.Fatalf("mkdir orphan: %v", err)
	}
	if err := os.WriteFile(filepath.Join(orphanDir, "old.html"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write orphan: %v", err)
	}

	d.cleanupRenders()

	if _, err := os.Stat(orphanDir); !os.IsNotExist(err) {
		t.Errorf("orphan render dir should be removed, stat err = %v", err)
	}
	if _, err := os.Stat(d.RenderDir(live.ID)); err != nil {
		t.Errorf("live session render dir should survive, stat err = %v", err)
	}
}

// TestCleanupRendersTTLSweepsOldFiles verifies old files are reclaimed while
// fresh ones survive, and an emptied dir is pruned.
func TestCleanupRendersTTLSweepsOldFiles(t *testing.T) {
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	agent, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	s1, _ := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "mixed"})
	s2, _ := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "all-old"})

	fresh := writeRenderFile(t, d, s1.ID, "fresh.html", 0)
	stale := writeRenderFile(t, d, s1.ID, "stale.html", renderTTL+time.Hour)
	onlyOld := writeRenderFile(t, d, s2.ID, "only-old.html", renderTTL+time.Hour)

	d.cleanupRenders()

	if _, err := os.Stat(fresh); err != nil {
		t.Errorf("fresh file should survive TTL sweep: %v", err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale file should be reclaimed, stat err = %v", err)
	}
	if _, err := os.Stat(onlyOld); !os.IsNotExist(err) {
		t.Errorf("old file should be reclaimed, stat err = %v", err)
	}
	// s2's dir held only the swept file → it should be pruned.
	if _, err := os.Stat(d.RenderDir(s2.ID)); !os.IsNotExist(err) {
		t.Errorf("emptied render dir should be pruned, stat err = %v", err)
	}
	// s1 still has the fresh file → its dir must remain.
	if _, err := os.Stat(d.RenderDir(s1.ID)); err != nil {
		t.Errorf("render dir with a surviving file should remain: %v", err)
	}
}
