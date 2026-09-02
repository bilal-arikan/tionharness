package conversation

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// debugSessionID creates a real session so the recorders have a session
// directory to append debug.jsonl into (the journal is a sidecar next to
// session.json, not a standalone file).
func debugSessionID(t *testing.T, d *db.DB) string {
	t.Helper()
	ctx := context.Background()
	agent, err := d.CreateAgent(ctx, db.Agent{Name: "A", Provider: "anthropic"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sess, err := d.CreateSession(ctx, db.Session{AgentID: agent.ID, Title: "T"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return sess.ID
}

func TestNativeCompactErrorKind(t *testing.T) {
	if got := nativeCompactErrorKind(nil); got != "" {
		t.Fatalf("nil error kind = %q, want empty", got)
	}
	if got := nativeCompactErrorKind(errNoNativeCompactor); got != "" {
		t.Fatalf("absent callback kind = %q, want empty", got)
	}
	if got := nativeCompactErrorKind(errors.New("cli refused")); got != "compaction_failed" {
		t.Fatalf("failure kind = %q, want compaction_failed", got)
	}
}

// TestRecordNativeCompactDebugWritesEvent pins the three decision names through
// the journal's redaction: an unknown name would be fingerprinted, so reading the
// value back verbatim is what proves the allow-list still covers them.
func TestRecordNativeCompactDebugWritesEvent(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sid := debugSessionID(t, d)
	m := NewManager()
	for _, name := range []string{"native_skipped", "claim_consumed", "native_fallback_rolling"} {
		m.recordNativeCompactDebug(d, sid, "A1", name, "")
	}
	evs, err := d.ReadDebugEvents(ctx, sid, db.DebugCompaction, 0)
	if err != nil {
		t.Fatalf("read debug: %v", err)
	}
	if len(evs) != 3 {
		t.Fatalf("compaction events = %d, want 3", len(evs))
	}
	for i, want := range []string{"native_skipped", "claim_consumed", "native_fallback_rolling"} {
		if evs[i].Name != want {
			t.Fatalf("event %d name = %q, want %q", i, evs[i].Name, want)
		}
	}
}

func TestRecordNativeCompactDebugKeepsErrorKind(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sid := debugSessionID(t, d)
	NewManager().recordNativeCompactDebug(d, sid, "A1", "claim_consumed", "compaction_failed")
	evs, err := d.ReadDebugEvents(ctx, sid, db.DebugCompaction, 0)
	if err != nil {
		t.Fatalf("read debug: %v", err)
	}
	if len(evs) != 1 || evs[0].ErrorKind != "compaction_failed" {
		t.Fatalf("events = %+v, want one carrying errorKind compaction_failed", evs)
	}
}

// TestRecordNativeCompactDebugNoSession keeps the recorder inert where the other
// recorders are: no store, or no session to attribute the decision to.
func TestRecordNativeCompactDebugNoSession(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sid := debugSessionID(t, d)
	m := NewManager()
	m.recordNativeCompactDebug(nil, sid, "A1", "native_skipped", "")
	m.recordNativeCompactDebug(d, "", "A1", "native_skipped", "")
	evs, err := d.ReadDebugEvents(context.Background(), sid, "", 0)
	if err != nil {
		t.Fatalf("read debug: %v", err)
	}
	if len(evs) != 0 {
		t.Fatalf("events = %d, want 0", len(evs))
	}
}
