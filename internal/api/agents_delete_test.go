package api

import (
	"context"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/workspace"
)

// agentDeleteFixture builds a workspace-shaped harness with a real store and the
// live chat-run registry, which is what agentRunning actually queries.
func agentDeleteFixture(t *testing.T) (*Server, *workspace.Workspace, db.Agent, db.Session) {
	t.Helper()
	ctx := context.Background()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	agent, err := database.CreateAgent(ctx, db.Agent{Name: "Ada", Provider: "anthropic"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sess, err := database.CreateSession(ctx, db.Session{AgentID: agent.ID, Title: "T"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	s := newTestServer()
	s.runs = newChatRuns()
	wsp := &workspace.Workspace{Meta: workspace.Meta{ID: "WS1"}, DB: database}
	return s, wsp, agent, sess
}

// TestAgentRunning_IdleAgentIsDeletable: with nothing in flight the guard must
// not block — a false positive here makes an agent permanently undeletable.
func TestAgentRunning_IdleAgentIsDeletable(t *testing.T) {
	s, wsp, agent, _ := agentDeleteFixture(t)
	if running, where := s.agentRunning(context.Background(), wsp, agent.ID); running {
		t.Fatalf("idle agent reported running (at %q)", where)
	}
}

// TestAgentRunning_TurnInOwnedSession: a live turn in a session the agent owns
// must block the delete — that is the whole point of the guard.
func TestAgentRunning_TurnInOwnedSession(t *testing.T) {
	s, wsp, agent, sess := agentDeleteFixture(t)
	s.runs.register("R1", sess.ID, wsp.ID, func() {})

	running, where := s.agentRunning(context.Background(), wsp, agent.ID)
	if !running {
		t.Fatal("agent with a live turn must report running")
	}
	if where != sess.ID {
		t.Fatalf("blamed %q, want the live session %q", where, sess.ID)
	}

	// …and once the turn unwinds it becomes deletable again.
	s.runs.unregister("R1")
	if running, _ := s.agentRunning(context.Background(), wsp, agent.ID); running {
		t.Fatal("agent still reported running after its turn ended")
	}
}

// TestAgentRunning_TurnInAnotherAgentsSession: a multi-agent thread is owned by
// one agent but answered by several, so participation — not ownership — is what
// makes an agent busy.
func TestAgentRunning_TurnInAnotherAgentsSession(t *testing.T) {
	ctx := context.Background()
	s, wsp, owner, sess := agentDeleteFixture(t)
	peer, _ := wsp.DB.CreateAgent(ctx, db.Agent{Name: "Bob", Provider: "anthropic"})

	// Bob joins the thread as a participant (this is what AddMessage records).
	if _, err := wsp.DB.AddMessage(ctx, db.Message{
		SessionID:  sess.ID,
		Role:       "assistant",
		Text:       "merhaba",
		AuthorKind: db.AuthorAgent,
		AuthorID:   peer.ID,
	}); err != nil {
		t.Fatalf("add message: %v", err)
	}
	s.runs.register("R1", sess.ID, wsp.ID, func() {})

	if running, _ := s.agentRunning(ctx, wsp, peer.ID); !running {
		t.Fatal("a participant answering in someone else's session must count as running")
	}
	if running, _ := s.agentRunning(ctx, wsp, owner.ID); !running {
		t.Fatal("the session owner must also count as running")
	}
}

// TestAgentRunning_OtherWorkspaceTurnDoesNotBlock: the chat-run registry is
// server-wide, so a turn in a DIFFERENT workspace must not make this agent look
// busy.
func TestAgentRunning_OtherWorkspaceTurnDoesNotBlock(t *testing.T) {
	s, wsp, agent, sess := agentDeleteFixture(t)
	s.runs.register("R1", sess.ID, "OTHER-WS", func() {})

	if running, where := s.agentRunning(context.Background(), wsp, agent.ID); running {
		t.Fatalf("a turn in another workspace blocked the delete (at %q)", where)
	}
}

// NOTE: agentRunning also consults ListRunningRuns (task runs carry an AgentID),
// but that branch is untested here on purpose: the store exposes no way to create
// a Run — they are legacy rows, only ever loaded from disk (see _Docs/08 on
// "Task Run ID'leri … legacy, artık üretilmiyor"). The branch is kept because
// workspaceRunning still reads the same source, so a store that does hold legacy
// running rows is handled consistently.
