package db

import (
	"context"
	"path/filepath"
	"testing"
)

// TestSessionRunStatePersists verifies that the run outcome written by the
// coordination layer survives a close→reopen of the store: RunState is what makes
// a finished worker distinguishable from a live one after a restart, so an
// in-memory-only value would defeat the whole point of the field.
func TestSessionRunStatePersists(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "store")
	d, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ag, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	sess, _ := d.CreateSession(ctx, Session{AgentID: ag.ID, Title: "T"})

	// A session that has never run a background turn carries no outcome.
	if sess.RunState != "" || sess.RunStateAt != 0 {
		t.Fatalf("fresh session must have empty run state; got %q at %d", sess.RunState, sess.RunStateAt)
	}

	if err := d.SetSessionRunState(ctx, sess.ID, "completed", 1700000000); err != nil {
		t.Fatalf("set run state: %v", err)
	}
	got, err := d.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got.RunState != "completed" || got.RunStateAt != 1700000000 {
		t.Fatalf("run state = %q at %d, want completed at 1700000000", got.RunState, got.RunStateAt)
	}

	// A later failed run overwrites it — the field is "how the LAST turn ended".
	if err := d.SetSessionRunState(ctx, sess.ID, "failed", 1700000500); err != nil {
		t.Fatalf("set run state (failed): %v", err)
	}

	// RunState must NOT bleed into State, the two-valued visibility field.
	got, _ = d.GetSession(ctx, sess.ID)
	if got.State == "failed" {
		t.Fatal("run outcome leaked into Session.State (visibility field)")
	}

	// Reload from disk: the value came from session.json, not memory.
	d2, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	reloaded, err := d2.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("get after reload: %v", err)
	}
	if reloaded.RunState != "failed" || reloaded.RunStateAt != 1700000500 {
		t.Fatalf("after reload run state = %q at %d, want failed at 1700000500", reloaded.RunState, reloaded.RunStateAt)
	}
}

// TestSessionStallNudgesPersists verifies the cumulative coordinator-stall counter
// increments and survives a reload — the in-memory slot streak resets on a clean
// coordination call, so only this tally can inform a later escalation tier.
func TestSessionStallNudgesPersists(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "store")
	d, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ag, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	sess, _ := d.CreateSession(ctx, Session{AgentID: ag.ID, Title: "T"})

	for want := 1; want <= 3; want++ {
		n, err := d.BumpSessionStallNudges(ctx, sess.ID)
		if err != nil {
			t.Fatalf("bump: %v", err)
		}
		if n != want {
			t.Fatalf("bump returned %d, want %d", n, want)
		}
	}

	d2, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	reloaded, err := d2.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("get after reload: %v", err)
	}
	if reloaded.StallNudges != 3 {
		t.Fatalf("after reload StallNudges = %d, want 3", reloaded.StallNudges)
	}
}
