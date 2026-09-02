package db

import (
	"context"
	"path/filepath"
	"testing"
)

// TestSessionHookOpsAndLockFreedom: every lifecycle change reaches the hook with
// the right op and previous value, and the callback runs with NO store lock held
// — it can read the store (and even mutate another session) without deadlocking.
func TestSessionHookOpsAndLockFreedom(t *testing.T) {
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer d.Close()
	ag, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})

	var events []SessionChangeEvent
	d.SetSessionHook(func(ev SessionChangeEvent) {
		// Re-entrancy proof: a read and a listing under the same DB from inside
		// the callback. Both take d.mu; a hook fired under the lock would hang here.
		if _, err := d.GetSession(ctx, ev.SessionID); err != nil && ev.Op != SessionOpDelete {
			t.Errorf("hook re-entry GetSession(%s) on %s: %v", ev.SessionID, ev.Op, err)
		}
		if _, err := d.ListSessions(ctx, ""); err != nil {
			t.Errorf("hook re-entry ListSessions on %s: %v", ev.Op, err)
		}
		events = append(events, ev)
	})

	sess, err := d.CreateSession(ctx, Session{AgentID: ag.ID, Title: "T"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := d.SetSessionState(ctx, sess.ID, "archived"); err != nil {
		t.Fatalf("state: %v", err)
	}
	if err := d.SetSessionRunState(ctx, sess.ID, "completed", 100); err != nil {
		t.Fatalf("runstate: %v", err)
	}
	// Reuse paths fire only when they actually create.
	first, _ := d.GetOrCreateSourceSession(ctx, "automation", "AUT1", ag.ID, "auto")
	again, _ := d.GetOrCreateSourceSession(ctx, "automation", "AUT1", ag.ID, "auto")
	if first.ID != again.ID {
		t.Fatalf("source session not reused: %s vs %s", first.ID, again.ID)
	}
	kind1, _ := d.GetOrCreateKindSession(ctx, ag.ID, "schedule", "sched")
	kind2, _ := d.GetOrCreateKindSession(ctx, ag.ID, "schedule", "sched")
	if kind1.ID != kind2.ID {
		t.Fatalf("kind session not reused: %s vs %s", kind1.ID, kind2.ID)
	}
	if err := d.DeleteSession(ctx, sess.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	want := []struct {
		op   string
		id   string
		prev string
	}{
		{SessionOpCreate, sess.ID, ""},
		{SessionOpState, sess.ID, "active"},
		{SessionOpRunState, sess.ID, ""},
		{SessionOpCreate, first.ID, ""},
		{SessionOpCreate, kind1.ID, ""},
		{SessionOpDelete, sess.ID, ""},
	}
	if len(events) != len(want) {
		ops := make([]string, 0, len(events))
		for _, ev := range events {
			ops = append(ops, ev.Op+":"+ev.SessionID)
		}
		t.Fatalf("got %d events %v, want %d", len(events), ops, len(want))
	}
	for i, w := range want {
		ev := events[i]
		if ev.Op != w.op || ev.SessionID != w.id {
			t.Fatalf("event %d = %s:%s, want %s:%s", i, ev.Op, ev.SessionID, w.op, w.id)
		}
		if ev.Session.ID != w.id {
			t.Fatalf("event %d carries session %q, want %q", i, ev.Session.ID, w.id)
		}
		switch w.op {
		case SessionOpState:
			if ev.PrevState != w.prev || ev.Session.State != "archived" {
				t.Fatalf("state event prev=%q new=%q, want %q→archived", ev.PrevState, ev.Session.State, w.prev)
			}
		case SessionOpRunState:
			if ev.PrevRunState != "" || ev.Session.RunState != "completed" {
				t.Fatalf("runstate event prev=%q new=%q, want \"\"→completed", ev.PrevRunState, ev.Session.RunState)
			}
		case SessionOpDelete:
			if ev.Session.Title != "T" {
				t.Fatalf("delete event must carry the removed row, got title %q", ev.Session.Title)
			}
		}
	}

	// Clearing the hook silences it.
	d.SetSessionHook(nil)
	n := len(events)
	if _, err := d.CreateSession(ctx, Session{AgentID: ag.ID}); err != nil {
		t.Fatalf("create after clear: %v", err)
	}
	if len(events) != n {
		t.Fatal("a cleared hook must not fire")
	}
}

// TestSessionHookNotFiredOnFailure: a rejected create leaves no event behind.
func TestSessionHookNotFiredOnFailure(t *testing.T) {
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer d.Close()
	fired := 0
	d.SetSessionHook(func(SessionChangeEvent) { fired++ })
	if _, err := d.CreateSession(ctx, Session{Origin: &SessionOrigin{Kind: "nope"}}); err == nil {
		t.Fatal("expected invalid origin to be rejected")
	}
	if _, err := d.CreateChildSession(ctx, Session{ParentSessionID: "SES-missing", ExecutionType: ExecutionSubagent}); err == nil {
		t.Fatal("expected missing parent to be rejected")
	}
	if fired != 0 {
		t.Fatalf("hook fired %d times on failed creates", fired)
	}
}
