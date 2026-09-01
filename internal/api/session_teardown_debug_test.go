package api

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

// teardownDebugWS is testWS plus a real store and a real session, so the
// lifecycle recorders have a session directory to append debug.jsonl into (the
// journal is a sidecar next to session.json, not a standalone file).
func teardownDebugWS(t *testing.T, id string) (*workspace.Workspace, string) {
	t.Helper()
	ctx := context.Background()
	d, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	agent, err := d.CreateAgent(ctx, db.Agent{Name: "A", Provider: "anthropic"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sess, err := d.CreateSession(ctx, db.Session{AgentID: agent.ID, Title: "T"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	wsp := testWS(id)
	wsp.DB = d
	return wsp, sess.ID
}

func lifecycleEvents(t *testing.T, wsp *workspace.Workspace, sessionID string) []db.DebugEvent {
	t.Helper()
	evs, err := wsp.DB.ReadDebugEvents(context.Background(), sessionID, db.DebugLifecycle, 0)
	if err != nil {
		t.Fatalf("read debug: %v", err)
	}
	return evs
}

// TestTeardown_JournalsCancelledChatTurn: a turn stopped by a delete just stops
// emitting, so without this event the transcript reads as if it died on its own.
func TestTeardown_JournalsCancelledChatTurn(t *testing.T) {
	s := &Server{inbox: newInboxStore(), runs: newChatRuns()}
	wsp, sid := teardownDebugWS(t, "WS1")
	s.runs.register("R1", sid, "WS1", func() { s.runs.unregister("R1") })

	if err := s.teardownSessionRuntimeWithGrace(wsp, sid, time.Second); err != nil {
		t.Fatalf("teardown returned error: %v", err)
	}
	evs := lifecycleEvents(t, wsp, sid)
	if len(evs) != 1 {
		t.Fatalf("lifecycle events = %+v, want exactly one", evs)
	}
	if evs[0].Name != "turn_cancelled_by_teardown" {
		t.Fatalf("name = %q, want turn_cancelled_by_teardown", evs[0].Name)
	}
}

// TestTeardown_JournalsGraceExceeded: an aborted delete surfaces only as the HTTP
// error the caller sees, so the journal is where the session itself records that
// its turn refused to stop.
func TestTeardown_JournalsGraceExceeded(t *testing.T) {
	s := &Server{inbox: newInboxStore(), runs: newChatRuns()}
	wsp, sid := teardownDebugWS(t, "WS1")
	s.runs.register("R1", sid, "WS1", func() {}) // never unregisters

	if err := s.teardownSessionRuntimeWithGrace(wsp, sid, 30*time.Millisecond); err == nil {
		t.Fatal("want an abort for a turn that never stops")
	}
	evs := lifecycleEvents(t, wsp, sid)
	if len(evs) != 1 {
		t.Fatalf("lifecycle events = %+v, want exactly one", evs)
	}
	if evs[0].Name != "teardown_grace_exceeded" {
		t.Fatalf("name = %q, want teardown_grace_exceeded", evs[0].Name)
	}
	if evs[0].DurationMs != (30 * time.Millisecond).Milliseconds() {
		t.Fatalf("durationMs = %d, want the grace window", evs[0].DurationMs)
	}
}

// TestTeardown_QuietSessionJournalsNothing keeps the recorders off the common
// path: deleting an idle session cancelled nothing, so it reports nothing.
func TestTeardown_QuietSessionJournalsNothing(t *testing.T) {
	s := &Server{inbox: newInboxStore(), runs: newChatRuns()}
	wsp, sid := teardownDebugWS(t, "WS1")

	if err := s.teardownSessionRuntimeWithGrace(wsp, sid, time.Second); err != nil {
		t.Fatalf("teardown returned error: %v", err)
	}
	if evs := lifecycleEvents(t, wsp, sid); len(evs) != 0 {
		t.Fatalf("lifecycle events = %+v, want none", evs)
	}
}

func TestNoteTeardownGraceIgnoresEarlyFailure(t *testing.T) {
	s := &Server{inbox: newInboxStore(), runs: newChatRuns()}
	wsp, sid := teardownDebugWS(t, "WS1")
	// A phase that failed for its own reason, well inside the window: not a grace
	// exhaustion, so it must not be journalled as one.
	s.noteTeardownGrace(wsp, sid, "worker", time.Now().Add(time.Minute), time.Minute)
	if evs := lifecycleEvents(t, wsp, sid); len(evs) != 0 {
		t.Fatalf("lifecycle events = %+v, want none", evs)
	}
}

func TestRecordQueuedTurnDropped(t *testing.T) {
	s := &Server{inbox: newInboxStore(), runs: newChatRuns()}
	wsp, sid := teardownDebugWS(t, "WS1")
	s.recordQueuedTurnDropped(wsp, sid, "user_cancel")
	s.recordQueuedTurnDropped(nil, sid, "queue_cleared") // no workspace → no event

	evs := lifecycleEvents(t, wsp, sid)
	if len(evs) != 1 {
		t.Fatalf("lifecycle events = %+v, want exactly one", evs)
	}
	if evs[0].Name != "queued_turn_dropped" {
		t.Fatalf("name = %q, want queued_turn_dropped", evs[0].Name)
	}
}
