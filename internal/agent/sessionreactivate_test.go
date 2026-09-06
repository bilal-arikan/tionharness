package agent

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/turnqueue"
)

// archivedFixture creates a session that is archived exactly the way the archive
// paths leave it: state "archived" PLUS the mirrored "archived" auto-tag, next to
// an unrelated tag that must survive the reactivation untouched.
func archivedFixture(t *testing.T, rt *Runtime) string {
	t.Helper()
	ctx := context.Background()
	agentRow, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Ada"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sess, err := rt.db.CreateSession(ctx, db.Session{Kind: "chat", AgentID: agentRow.ID})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := rt.db.SetSessionState(ctx, sess.ID, "archived"); err != nil {
		t.Fatalf("archive session: %v", err)
	}
	if err := rt.db.SetSessionTags(ctx, sess.ID, []string{"keep-me", TagArchived}); err != nil {
		t.Fatalf("tag session: %v", err)
	}
	return sess.ID
}

func sessionTags(t *testing.T, rt *Runtime, sessionID string) (string, []string) {
	t.Helper()
	sess, err := rt.db.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("read session: %v", err)
	}
	return sess.State, sess.Tags
}

// TestArchivedSessionReactivatedByIncomingTurn: sending a real command or message
// to an archived session lifts it back into the active list — a user message, a
// peer send_message, a send_to_worker task and a spawn's opening turn all count —
// and the mirrored "archived" tag goes with the state while other tags stay.
func TestArchivedSessionReactivatedByIncomingTurn(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))

	for _, tc := range []struct {
		name  string
		claim func(string) func()
	}{
		{"user message", func(id string) func() { return rt.BeginSessionUserTurn(id) }},
		{"peer message", func(id string) func() {
			return rt.claimSessionTurnSlot(id, turnqueue.KindPeer, "ajan mesajı")
		}},
		{"worker task", func(id string) func() {
			return rt.claimSessionTurnSlot(id, turnqueue.KindWorker, "worker görevi")
		}},
		{"spawn opening turn", func(id string) func() {
			return rt.claimSessionTurnSlot(id, turnqueue.KindSpawn, "spawn turu")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := archivedFixture(t, rt)
			tc.claim(id)()

			state, tags := sessionTags(t, rt, id)
			if state != "active" {
				t.Fatalf("state = %q, want %q — an inbound turn must un-archive the session", state, "active")
			}
			if containsTag(tags, TagArchived) {
				t.Fatalf("the %q tag must be dropped with the state, tags = %v", TagArchived, tags)
			}
			if !containsTag(tags, "keep-me") {
				t.Fatalf("unrelated tags must survive reactivation, tags = %v", tags)
			}
		})
	}
}

// TestArchivedSessionSurvivesReadOnlyAccess: only a genuine inbound message
// reactivates. Reading a session (opening it, listing its transcript) is not an
// instruction to the agent, so the session stays archived and keeps its tag.
func TestArchivedSessionSurvivesReadOnlyAccess(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	id := archivedFixture(t, rt)

	if _, err := rt.db.GetSession(ctx, id); err != nil {
		t.Fatalf("get session: %v", err)
	}
	if _, err := rt.db.ListMessages(ctx, id); err != nil {
		t.Fatalf("list messages: %v", err)
	}

	state, tags := sessionTags(t, rt, id)
	if state != "archived" {
		t.Fatalf("state = %q, want %q — read-only access must not reactivate", state, "archived")
	}
	if !containsTag(tags, TagArchived) {
		t.Fatalf("the %q tag must survive read-only access, tags = %v", TagArchived, tags)
	}
}

// TestArchivedSessionSurvivesMaintenanceAndAutomaticTurns: archiving is the hard
// stop on automatic work. A maintenance slash command, a coordinator drain, a
// scheduler wake and an automation trigger all claim the same turn slot, and none
// of them may bring the session back — otherwise archiving a runaway coordinator
// would no longer break its loop and a scheduled session could never stay archived.
func TestArchivedSessionSurvivesMaintenanceAndAutomaticTurns(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))

	for _, tc := range []struct {
		name  string
		claim func(string) func()
	}{
		{"slash command", func(id string) func() {
			release, err := rt.ClaimSessionCommandTurn(context.Background(), id, "/compact")
			if err != nil {
				t.Fatalf("claim command turn: %v", err)
			}
			return release
		}},
		{"coordinator drain", func(id string) func() {
			return rt.claimSessionTurnSlot(id, turnqueue.KindCoordinator, "worker bildirimi")
		}},
		{"scheduler wake", func(id string) func() {
			return rt.claimSessionTurnSlot(id, turnqueue.KindWake, "uyandırma")
		}},
		{"automation trigger", func(id string) func() {
			return rt.claimSessionTurnSlot(id, turnqueue.KindAutomation, "otomasyon tetiği")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := archivedFixture(t, rt)
			tc.claim(id)()

			state, tags := sessionTags(t, rt, id)
			if state != "archived" {
				t.Fatalf("state = %q, want %q — an automatic turn must not reactivate", state, "archived")
			}
			if !containsTag(tags, TagArchived) {
				t.Fatalf("the %q tag must survive an automatic turn, tags = %v", TagArchived, tags)
			}
		})
	}
}

// TestActiveSessionUntouchedByIncomingTurn: the reactivation hook is a no-op on a
// live session — it must not rewrite state or strip tags on the common path.
func TestActiveSessionUntouchedByIncomingTurn(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()

	agentRow, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Ada"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sess, err := rt.db.CreateSession(ctx, db.Session{Kind: "chat", AgentID: agentRow.ID})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := rt.db.SetSessionTags(ctx, sess.ID, []string{"keep-me"}); err != nil {
		t.Fatalf("tag session: %v", err)
	}

	rt.BeginSessionUserTurn(sess.ID)()

	state, tags := sessionTags(t, rt, sess.ID)
	if state == "archived" {
		t.Fatalf("state = %q, want a live session to stay live", state)
	}
	if !containsTag(tags, "keep-me") {
		t.Fatalf("tags must be untouched on a live session, tags = %v", tags)
	}
}
