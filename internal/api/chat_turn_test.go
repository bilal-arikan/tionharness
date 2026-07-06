package api

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

// TestSessionStateBlock verifies the <session_state> block carries session +
// workspace identity and resolves an empty permission mode to "auto".
func TestSessionStateBlock(t *testing.T) {
	got := sessionStateBlock(
		db.Session{ID: "SES9"},
		db.Agent{PermissionMode: "read-only"},
		"WS1", "My Space", "/data/ws1",
	)
	for _, want := range []string{"<session_state>", "sessionId: SES9", "permissionMode: read-only", "workspace: WS1 \"My Space\"", "workspacePath: /data/ws1", "</session_state>"} {
		if !strings.Contains(got, want) {
			t.Fatalf("session_state block missing %q\n%s", want, got)
		}
	}
	// Empty permission mode defaults to auto.
	if def := sessionStateBlock(db.Session{ID: "S"}, db.Agent{}, "WS1", "n", "/p"); !strings.Contains(def, "permissionMode: auto") {
		t.Fatalf("empty permission mode must default to auto, got:\n%s", def)
	}
}

// newTestServer builds a Server with just a discarding logger — enough for the
// turn-routing helpers that only touch the DB and the logger.
func newTestServer() *Server {
	return &Server{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

// TestResolveTurnAgents covers @mention routing: requested ids resolve to agents
// in order, duplicates and unknown ids drop out, and an empty/all-invalid list
// falls back to the session's own default agent.
func TestResolveTurnAgents(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()
	s := newTestServer()

	a1, _ := database.CreateAgent(ctx, db.Agent{Name: "Alice"})
	a2, _ := database.CreateAgent(ctx, db.Agent{Name: "Bob"})
	sess, _ := database.CreateSession(ctx, db.Session{AgentID: a1.ID})

	// Ordered + de-duplicated; unknown ids are skipped.
	got := s.resolveTurnAgents(ctx, database, sess, []string{a2.ID, a1.ID, a2.ID, "nope"})
	if len(got) != 2 || got[0].ID != a2.ID || got[1].ID != a1.ID {
		t.Fatalf("expected [Bob, Alice], got %+v", got)
	}

	// No ids → fall back to the session's default agent.
	got = s.resolveTurnAgents(ctx, database, sess, nil)
	if len(got) != 1 || got[0].ID != a1.ID {
		t.Fatalf("expected fallback to session agent, got %+v", got)
	}

	// All-invalid ids → still fall back to the session's default agent.
	got = s.resolveTurnAgents(ctx, database, sess, []string{"x", ""})
	if len(got) != 1 || got[0].ID != a1.ID {
		t.Fatalf("expected fallback for invalid ids, got %+v", got)
	}
}

// TestAdoptMentionedAgent verifies a brand-new session opened by @mentioning an
// agent pins the whole thread to that agent, while established sessions and
// no-mention turns are left untouched.
func TestAdoptMentionedAgent(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()
	s := newTestServer()

	def, _ := database.CreateAgent(ctx, db.Agent{Name: "Default"})
	other, _ := database.CreateAgent(ctx, db.Agent{Name: "Other"})

	// Fresh session (MessageCount 0) + a mention → adopt the first mentioned agent,
	// both in the returned struct and persisted to the DB.
	fresh, _ := database.CreateSession(ctx, db.Session{AgentID: def.ID})
	out := s.adoptMentionedAgent(ctx, database, fresh, []string{other.ID}, []db.Agent{other})
	if out.AgentID != other.ID {
		t.Fatalf("expected adopted agent %s, got %s", other.ID, out.AgentID)
	}
	if reloaded, _ := database.GetSession(ctx, fresh.ID); reloaded.AgentID != other.ID {
		t.Fatalf("expected persisted agent %s, got %s", other.ID, reloaded.AgentID)
	}

	// Established session (has messages) → never re-pinned.
	used, _ := database.CreateSession(ctx, db.Session{AgentID: def.ID})
	if _, err := database.AddMessage(ctx, db.Message{SessionID: used.ID, Role: "user", Text: "hi"}); err != nil {
		t.Fatalf("add message: %v", err)
	}
	used, _ = database.GetSession(ctx, used.ID)
	out = s.adoptMentionedAgent(ctx, database, used, []string{other.ID}, []db.Agent{other})
	if out.AgentID != def.ID {
		t.Fatalf("established session must not be re-pinned, got %s", out.AgentID)
	}

	// No mention (empty ids) → no-op even on a fresh session.
	noMention, _ := database.CreateSession(ctx, db.Session{AgentID: def.ID})
	out = s.adoptMentionedAgent(ctx, database, noMention, nil, nil)
	if out.AgentID != def.ID {
		t.Fatalf("no-mention turn must not change agent, got %s", out.AgentID)
	}

	// Mention resolves to the agent that is already the default → no redundant write.
	same, _ := database.CreateSession(ctx, db.Session{AgentID: def.ID})
	out = s.adoptMentionedAgent(ctx, database, same, []string{def.ID}, []db.Agent{def})
	if out.AgentID != def.ID {
		t.Fatalf("expected unchanged default agent, got %s", out.AgentID)
	}
}
