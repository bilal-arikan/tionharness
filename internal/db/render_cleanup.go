package db

import (
	"os"
	"path/filepath"
	"time"
)

// renderTTL bounds how long a render_template output file lingers before the
// startup sweep reclaims it. Render outputs are TRANSIENT chat-render views — a
// durable deliverable is saved as an artifact instead — so a swept file only
// makes its html-preview show "not found" until the model re-renders, an
// acceptable trade for not letting the render tree grow without bound.
const renderTTL = 14 * 24 * time.Hour

// RenderDir returns the absolute per-session directory where render_template
// writes filled HTML output: <root>/render/<sessionID>. Single source of truth
// shared by the agent runtime (which writes here via Runtime.SessionRenderDir)
// and the cleanup paths (session delete + the startup sweep), so the location can
// never drift between writer and reclaimer.
func (d *DB) RenderDir(sessionID string) string {
	return d.dir(dirRender, sessionID)
}

// cleanupRenders reclaims render_template output. It runs once on Open (before any
// concurrency, matching the migration sweeps) and:
//   - removes any <root>/render/<sid> whose session no longer exists (orphaned by
//     a crash before DeleteSession, or an older delete that predated this sweep), and
//   - deletes files older than renderTTL under a live-session dir, pruning that
//     dir when it ends up empty.
//
// Best-effort: every error is skipped so a single unreadable entry cannot block
// startup. Reads d.sessions directly without locking — startup is single-threaded
// here, exactly like cleanupOrphanUploads.
func (d *DB) cleanupRenders() {
	renderRoot := d.dir(dirRender)
	entries, err := os.ReadDir(renderRoot)
	if err != nil {
		return // no render tree yet — nothing to sweep
	}

	live := make(map[string]bool, len(d.sessions))
	for id := range d.sessions {
		live[id] = true
	}

	cutoff := time.Now().Add(-renderTTL)
	for _, e := range entries {
		if !e.IsDir() {
			continue // a stray file at the render root — leave it alone
		}
		sid := e.Name()
		dir := filepath.Join(renderRoot, sid)
		if !live[sid] {
			_ = os.RemoveAll(dir) // orphan: session gone, drop the whole dir
			continue
		}
		// Live session: TTL-sweep old files, then prune the dir if it emptied.
		files, ferr := os.ReadDir(dir)
		if ferr != nil {
			continue
		}
		remaining := 0
		for _, f := range files {
			if f.IsDir() {
				remaining++
				continue
			}
			info, ierr := f.Info()
			if ierr != nil {
				remaining++
				continue
			}
			if info.ModTime().Before(cutoff) && os.Remove(filepath.Join(dir, f.Name())) == nil {
				continue // reclaimed
			}
			remaining++
		}
		if remaining == 0 {
			_ = os.Remove(dir)
		}
	}
}
