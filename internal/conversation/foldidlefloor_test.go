package conversation

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

func foldFloorStore(t *testing.T) (*db.DB, string) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return d, debugSessionID(t, d)
}

func foldFloorEvents(t *testing.T, d *db.DB, sessionID string) []db.DebugEvent {
	t.Helper()
	evs, err := d.ReadDebugEvents(context.Background(), sessionID, db.DebugPressure, 0)
	if err != nil {
		t.Fatalf("read debug: %v", err)
	}
	return evs
}

// TestRecordFoldIdleFloorDebugRecordsStall covers the case the event exists for:
// the fold outlived the ordinary watchdog window, so only FoldIdleOutputFloor
// kept it alive.
func TestRecordFoldIdleFloorDebugRecordsStall(t *testing.T) {
	global := providers.StdoutIdleWindow()
	if global <= 0 || global >= FoldIdleOutputFloor {
		t.Skipf("stdout idle watchdog (%s) leaves no room below the fold floor", global)
	}
	d, sid := foldFloorStore(t)
	NewManager().recordFoldIdleFloorDebug(d, sid, "A1", global+time.Minute)
	evs := foldFloorEvents(t, d, sid)
	if len(evs) != 1 {
		t.Fatalf("pressure events = %d, want 1", len(evs))
	}
	if evs[0].Name != "fold_idle_floor" {
		t.Fatalf("name = %q, want fold_idle_floor", evs[0].Name)
	}
	if want := (global + time.Minute).Milliseconds(); evs[0].DurationMs != want {
		t.Fatalf("durationMs = %d, want %d", evs[0].DurationMs, want)
	}
}

// TestRecordFoldIdleFloorDebugSilentInsideWindow keeps an ordinary fold — one the
// watchdog would never have touched — out of the journal.
func TestRecordFoldIdleFloorDebugSilentInsideWindow(t *testing.T) {
	global := providers.StdoutIdleWindow()
	if global <= 0 {
		t.Skip("stdout idle watchdog disabled")
	}
	d, sid := foldFloorStore(t)
	NewManager().recordFoldIdleFloorDebug(d, sid, "A1", global-time.Second)
	if evs := foldFloorEvents(t, d, sid); len(evs) != 0 {
		t.Fatalf("pressure events = %d, want 0", len(evs))
	}
}

func TestRecordFoldIdleFloorDebugNoSession(t *testing.T) {
	d, sid := foldFloorStore(t)
	NewManager().recordFoldIdleFloorDebug(d, "", "A1", time.Hour)
	if evs := foldFloorEvents(t, d, sid); len(evs) != 0 {
		t.Fatalf("pressure events = %d, want 0", len(evs))
	}
}
