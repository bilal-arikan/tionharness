package agent

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

func busyFixture(t *testing.T) (*Runtime, db.Agent, db.Session) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	rt := runtimeOverStore(t, filepath.Join(dir, "store"), filepath.Join(dir, "work"))

	agent, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Ada", Provider: "anthropic"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sess, err := rt.db.CreateSession(ctx, db.Session{AgentID: agent.ID, Title: "T"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return rt, agent, sess
}

// TestAgentBusy_IdleIsNotBusy: a false positive makes an agent permanently
// undeletable, so the quiet case must stay quiet.
func TestAgentBusy_IdleIsNotBusy(t *testing.T) {
	rt, agent, _ := busyFixture(t)
	if busy, where := rt.AgentBusy(context.Background(), agent.ID); busy {
		t.Fatalf("idle agent reported busy (at %q)", where)
	}
}

// TestAgentBusy_OwnAutonomousSession: the runtime's own tracking (schedule wake,
// spawn, inbox delivery, flow nodes) must count.
func TestAgentBusy_OwnAutonomousSession(t *testing.T) {
	rt, agent, sess := busyFixture(t)
	rt.trackSession(sess.ID, func() {})

	busy, where := rt.AgentBusy(context.Background(), agent.ID)
	if !busy {
		t.Fatal("agent with an autonomous session must report busy")
	}
	if where != sess.ID {
		t.Fatalf("blamed %q, want %q", where, sess.ID)
	}

	rt.untrackSession(sess.ID)
	if busy, _ := rt.AgentBusy(context.Background(), agent.ID); busy {
		t.Fatal("still busy after the session was untracked")
	}
}

// TestAgentBusy_ExternalProbeCounts is the whole point of the injection: the
// INTERACTIVE chat runs live in the api server, invisible to this runtime. Before
// the probe was wired, the delete_agent tool saw only autonomous work and would
// happily delete an agent mid-chat.
func TestAgentBusy_ExternalProbeCounts(t *testing.T) {
	rt, agent, sess := busyFixture(t)
	if busy, _ := rt.AgentBusy(context.Background(), agent.ID); busy {
		t.Fatal("precondition: agent should be idle without the probe")
	}

	rt.SetExternalActiveSessions(func() []string { return []string{sess.ID} })

	busy, where := rt.AgentBusy(context.Background(), agent.ID)
	if !busy {
		t.Fatal("an interactive turn reported by the probe must count as busy")
	}
	if where != sess.ID {
		t.Fatalf("blamed %q, want %q", where, sess.ID)
	}
}

// TestAgentBusy_ParticipantCounts: in a multi-agent thread the responder is often
// a participant, not the owner — ownership alone would miss it.
func TestAgentBusy_ParticipantCounts(t *testing.T) {
	ctx := context.Background()
	rt, owner, sess := busyFixture(t)
	peer, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Bob", Provider: "anthropic"})
	if err != nil {
		t.Fatalf("create peer: %v", err)
	}
	if _, err := rt.db.AddMessage(ctx, db.Message{
		SessionID:  sess.ID,
		Role:       "assistant",
		Text:       "merhaba",
		AuthorKind: db.AuthorAgent,
		AuthorID:   peer.ID,
	}); err != nil {
		t.Fatalf("add message: %v", err)
	}
	rt.trackSession(sess.ID, func() {})

	if busy, _ := rt.AgentBusy(ctx, peer.ID); !busy {
		t.Fatal("a participant answering in someone else's session must be busy")
	}
	if busy, _ := rt.AgentBusy(ctx, owner.ID); !busy {
		t.Fatal("the session owner must be busy too")
	}
}

// TestAgentBusy_UnrelatedSessionDoesNotBlock: a live session belonging to another
// agent must not make this one undeletable.
func TestAgentBusy_UnrelatedSessionDoesNotBlock(t *testing.T) {
	ctx := context.Background()
	rt, _, sess := busyFixture(t)
	other, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Other", Provider: "anthropic"})
	if err != nil {
		t.Fatalf("create other: %v", err)
	}
	rt.trackSession(sess.ID, func() {})

	if busy, where := rt.AgentBusy(ctx, other.ID); busy {
		t.Fatalf("unrelated agent reported busy (at %q)", where)
	}
}

// TestAgentBusy_NilSafe: a nil runtime / empty id must not panic — AgentBusy is
// reached from the api layer during early boot too.
func TestAgentBusy_NilSafe(t *testing.T) {
	var rt *Runtime
	if busy, _ := rt.AgentBusy(context.Background(), "AGT1"); busy {
		t.Fatal("nil runtime must report not busy")
	}
	rt2, _, _ := busyFixture(t)
	if busy, _ := rt2.AgentBusy(context.Background(), ""); busy {
		t.Fatal("empty agent id must report not busy")
	}
}
