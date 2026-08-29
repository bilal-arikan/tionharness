package api

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// TestGuardCoordinatorChatTurn verifies the chat path hands coordinator turns to the
// stall guard — and only coordinator turns.
//
// The observable effect used here is the guard's recovery branch: a turn that DID call
// a coordination tool clears the persisted stall tally. It exercises the same call site
// as the phantom-spawn branch without needing a judge model, so the test is hermetic.
func TestGuardCoordinatorChatTurn(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rt := agent.NewRuntime(database, providers.NewRegistry(), agent.NewTunables(),
		t.TempDir(), t.TempDir(), nil, nil, "WS1", "Test", nil, logger)
	s := newTestServer()

	agentRow, err := database.CreateAgent(ctx, db.Agent{Name: "Coord", Provider: "anthropic", Model: "m"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	steps := []agent.TurnStep{{Kind: agent.StepTool, Tool: "spawn_worker"}}

	coord, err := database.CreateSession(ctx, db.Session{AgentID: agentRow.ID, Kind: "chat", CoordinatorMode: true})
	if err != nil {
		t.Fatalf("create coordinator session: %v", err)
	}
	if err := database.SetSessionStallNudges(ctx, coord.ID, 3); err != nil {
		t.Fatalf("seed stall tally: %v", err)
	}
	s.guardCoordinatorChatTurn(ctx, database, rt, coord.ID, agentRow, "spawning", steps)
	got, err := database.GetSession(ctx, coord.ID)
	if err != nil {
		t.Fatalf("get coordinator session: %v", err)
	}
	if got.StallNudges != 0 {
		t.Fatalf("coordinator chat turn was not passed to the stall guard; tally still %d", got.StallNudges)
	}

	// A plain session must not be guarded at all: its tally is untouched.
	plain, err := database.CreateSession(ctx, db.Session{AgentID: agentRow.ID, Kind: "chat"})
	if err != nil {
		t.Fatalf("create plain session: %v", err)
	}
	if err := database.SetSessionStallNudges(ctx, plain.ID, 3); err != nil {
		t.Fatalf("seed stall tally: %v", err)
	}
	s.guardCoordinatorChatTurn(ctx, database, rt, plain.ID, agentRow, "spawning", steps)
	got, err = database.GetSession(ctx, plain.ID)
	if err != nil {
		t.Fatalf("get plain session: %v", err)
	}
	if got.StallNudges != 3 {
		t.Fatalf("a non-coordinator session must not be guarded; tally changed to %d", got.StallNudges)
	}
}
