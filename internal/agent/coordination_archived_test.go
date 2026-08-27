package agent

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestArchivedCoordinatorRefusesTurn locks the 2026-08-27 fix: archiving a
// coordinator must stop AUTOMATIC turns at the wake entry point, not only in
// RecoverOrphanedTurns. Before the guard, archiving the three runaway coordinators
// in a workspace was not enough — their orphaned workers were still reclaimed at
// boot, and every reclaim called NotifyCoordinator, which started a fresh drain on
// a session the user had explicitly archived.
func TestArchivedCoordinatorRefusesTurn(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	coord := newTestCoordinator(t, rt, 0)

	var mu sync.Mutex
	turns := 0
	rt.coordRunFn = func(string) {
		mu.Lock()
		turns++
		mu.Unlock()
	}

	if err := rt.db.SetSessionState(context.Background(), coord, "archived"); err != nil {
		t.Fatalf("archive coordinator session: %v", err)
	}

	// This is the exact path a reclaimed worker takes at boot.
	rt.NotifyCoordinator(coord, "worker died mid-turn")

	// The guard returns synchronously, but give a would-be drain goroutine room to
	// run so the test fails loudly instead of racing to a false pass.
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	got := turns
	mu.Unlock()
	if got != 0 {
		t.Fatalf("archived coordinator must not run automatic turns; ran %d", got)
	}

	// The slot must stay untouched too — a drain that never started cannot be left
	// flagged as driving, or un-archiving would deadlock the session.
	slot := rt.coordSlotFor(coord)
	slot.mu.Lock()
	driving, pending := slot.driving, slot.pending
	slot.mu.Unlock()
	if driving || pending {
		t.Fatalf("archived coordinator left the slot armed; driving=%v pending=%v", driving, pending)
	}

	// Un-archiving restores normal behaviour: the note the worker recorded is still
	// in history, and the next notification drains it.
	if err := rt.db.SetSessionState(context.Background(), coord, "active"); err != nil {
		t.Fatalf("restore coordinator session: %v", err)
	}
	rt.NotifyCoordinator(coord, "retry after restore")

	deadline := time.After(2 * time.Second)
	for {
		mu.Lock()
		got = turns
		mu.Unlock()
		if got > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("un-archived coordinator never ran a turn")
		case <-time.After(5 * time.Millisecond):
		}
	}
}
