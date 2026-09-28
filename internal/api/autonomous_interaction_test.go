package api

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// newAutonomousTestServer builds the minimal Server + Runtime pair
// autonomousInteraction needs: a non-empty selfURL (otherwise the setup returns
// early), the run registry and the per-session grant store.
func newAutonomousTestServer(t *testing.T) (*Server, *agent.Runtime) {
	t.Helper()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rt := agent.NewRuntime(database, providers.NewRegistry(), agent.NewTunables(), t.TempDir(), t.TempDir(), nil, nil, "WS1", "Test", nil, logger)
	t.Cleanup(rt.CloseMCP)
	s := &Server{
		logger:  logger,
		tun:     agent.NewTunables(),
		runs:    newChatRuns(),
		grants:  newPermGrantStore(),
		selfURL: "http://127.0.0.1:8090",
	}
	return s, rt
}

// activeRun returns the single run autonomousInteraction registered.
func activeRun(t *testing.T, s *Server) *chatRun {
	t.Helper()
	s.runs.mu.Lock()
	defer s.runs.mu.Unlock()
	if len(s.runs.runs) != 1 {
		t.Fatalf("registered runs = %d, want 1", len(s.runs.runs))
	}
	for _, r := range s.runs.runs {
		return r
	}
	return nil
}

// The autonomous bridge carries steering in every permission mode.
func TestAutonomousInteractionRecordsSteerable(t *testing.T) {
	cases := []struct {
		provider string
		mode     string
		want     bool
	}{
		{"claude-cli", "ask", true},
		{"claude-cli", "read-only", true},
		{"claude-cli", "auto", true},
		{"codex-cli", "ask", true},
		{"codex-cli", "auto", true},
		{"codex-cli", "read-only", true},
		{"anthropic", "auto", true},
	}
	for _, tc := range cases {
		s, rt := newAutonomousTestServer(t)
		ag := db.Agent{ID: "AG1", Provider: tc.provider, PermissionMode: tc.mode}
		_, done := s.autonomousInteraction(rt)(context.Background(), ag, "SES1")
		got := activeRun(t, s).steerableFor()
		if got != tc.want {
			t.Fatalf("provider=%s mode=%s: steerable = %v, want %v", tc.provider, tc.mode, got, tc.want)
		}
		done()
	}
}

// TestAutonomousInteractionBindsGrants verifies the autonomous turn binds the
// session-scoped permission grants onto the run and the context. A nil grant store
// made grantSkillToolsCLI bail out, silently killing SK-3's allowed-tools
// auto-grant, and dropped every "Always allow" decision in ask mode.
func TestAutonomousInteractionBindsGrants(t *testing.T) {
	s, rt := newAutonomousTestServer(t)
	ag := db.Agent{ID: "AG1", Provider: "claude-cli", PermissionMode: "ask"}
	ctx, done := s.autonomousInteraction(rt)(context.Background(), ag, "SES1")
	defer done()

	grants := activeRun(t, s).grantStore()
	if grants == nil {
		t.Fatal("autonomous run has no grant store: SK-3 auto-grant and 'Always allow' are dead")
	}
	// Same instance the chat path would hand out for this (workspace, session).
	if want := s.grants.forSession(rt.WorkspaceID(), "SES1"); grants != want {
		t.Fatal("autonomous run bound a different grant set than the session's")
	}
	if tools.GrantsFrom(ctx) != grants {
		t.Fatal("turn context does not carry the session grants")
	}
}

// TestAutonomousInteractionWithoutSessionSkipsGrants keeps the sessionID gate
// honest: with no session to scope them to, no grant set is bound (the nil store
// is handled defensively downstream).
func TestAutonomousInteractionWithoutSessionSkipsGrants(t *testing.T) {
	s, rt := newAutonomousTestServer(t)
	ag := db.Agent{ID: "AG1", Provider: "claude-cli", PermissionMode: "ask"}
	ctx, done := s.autonomousInteraction(rt)(context.Background(), ag, "")
	defer done()

	if activeRun(t, s).grantStore() != nil {
		t.Fatal("grant store bound for a session-less turn")
	}
	if tools.GrantsFrom(ctx) != nil {
		t.Fatal("context carries grants for a session-less turn")
	}
}
