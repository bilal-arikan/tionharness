package db

import (
	"context"
	"path/filepath"
	"testing"
)

// newDebugPolicySession opens a fresh store and returns it together with a
// session id whose folder exists, so the debug journal can be appended to.
func newDebugPolicySession(t *testing.T) (*DB, string) {
	t.Helper()
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	ctx := context.Background()
	agent, err := d.CreateAgent(ctx, Agent{Name: "A"})
	if err != nil {
		t.Fatal(err)
	}
	session, err := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "T"})
	if err != nil {
		t.Fatal(err)
	}
	return d, session.ID
}

func readPolicyEvents(t *testing.T, d *DB, sessionID string) []DebugEvent {
	t.Helper()
	evs, err := d.ReadDebugEvents(context.Background(), sessionID, DebugTool, 0)
	if err != nil {
		t.Fatalf("read debug events: %v", err)
	}
	return evs
}

// TestDebugJournalGateZeroValueEnabled pins the zero-value contract: a store
// nobody configured behaves as "enabled with the default cap", so an emit point
// that moved onto the gated helper keeps writing exactly as it did before.
func TestDebugJournalGateZeroValueEnabled(t *testing.T) {
	d, sessionID := newDebugPolicySession(t)

	if !d.DebugJournalEnabled() {
		t.Fatal("an unconfigured store must report the debug journal as enabled")
	}
	if got := d.DebugJournalCap(); got != DefaultDebugJournalCap {
		t.Fatalf("unconfigured cap = %d, want the default %d", got, DefaultDebugJournalCap)
	}

	if err := d.AppendDebugEventGated(sessionID, DebugEvent{Type: DebugTool, Out: 1}); err != nil {
		t.Fatalf("append gated event: %v", err)
	}
	evs := readPolicyEvents(t, d, sessionID)
	if len(evs) != 1 || evs[0].Out != 1 {
		t.Fatalf("unconfigured store must persist the event, got %+v", evs)
	}
	if sum, err := d.GetDebugSummary(context.Background(), sessionID); err != nil {
		t.Fatalf("summary: %v", err)
	} else if sum.ToolCalls != 1 {
		t.Fatalf("summary must see the event, got %d tool calls", sum.ToolCalls)
	}
}

// TestDebugJournalGateDisabledDropsEvent verifies the gate is a SILENT drop: the
// event never reaches disk, and the caller gets no error (emit points log their
// own failures, so returning one here would report a user setting as a fault).
func TestDebugJournalGateDisabledDropsEvent(t *testing.T) {
	d, sessionID := newDebugPolicySession(t)
	d.SetDebugJournal(false, 0)

	if d.DebugJournalEnabled() {
		t.Fatal("SetDebugJournal(false, …) must disable the journal")
	}
	if err := d.AppendDebugEventGated(sessionID, DebugEvent{Type: DebugTool, Out: 1}); err != nil {
		t.Fatalf("a disabled journal must drop silently, got error: %v", err)
	}
	if evs := readPolicyEvents(t, d, sessionID); len(evs) != 0 {
		t.Fatalf("disabled journal wrote %d events: %+v", len(evs), evs)
	}

	// Re-enabling restores writes — the gate is state, not a one-way switch.
	d.SetDebugJournal(true, 0)
	if err := d.AppendDebugEventGated(sessionID, DebugEvent{Type: DebugTool, Out: 2}); err != nil {
		t.Fatalf("append after re-enable: %v", err)
	}
	if evs := readPolicyEvents(t, d, sessionID); len(evs) != 1 || evs[0].Out != 2 {
		t.Fatalf("re-enabled journal must persist the event, got %+v", evs)
	}
}

// TestDebugJournalGateCapApplied verifies the configured cap reaches the pruning
// path. The journal prunes lazily — it rewrites down to cap once it drifts past
// cap + cap/4 — so the contract is "never more than cap + cap/4 retained, newest
// kept", not an exact count after every append.
func TestDebugJournalGateCapApplied(t *testing.T) {
	d, sessionID := newDebugPolicySession(t)
	const cap = 4
	d.SetDebugJournal(true, cap)

	if got := d.DebugJournalCap(); got != cap {
		t.Fatalf("configured cap = %d, want %d", got, cap)
	}

	const total = 12
	for i := 1; i <= total; i++ {
		if err := d.AppendDebugEventGated(sessionID, DebugEvent{Type: DebugTool, Out: i}); err != nil {
			t.Fatalf("append event %d: %v", i, err)
		}
	}

	evs := readPolicyEvents(t, d, sessionID)
	if len(evs) == 0 {
		t.Fatal("capped journal kept nothing")
	}
	if len(evs) > cap+cap/4 {
		t.Fatalf("capped journal retained %d events, want at most %d", len(evs), cap+cap/4)
	}
	if last := evs[len(evs)-1]; last.Out != total {
		t.Fatalf("pruning must keep the NEWEST events, last retained = %d, want %d", last.Out, total)
	}
	// The default cap would have kept all of them: the retained count is the proof
	// the configured value, not the default, drove the prune.
	if len(evs) >= total {
		t.Fatalf("configured cap was not applied: %d events retained out of %d", len(evs), total)
	}
}
