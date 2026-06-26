package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/swarmgo/internal/db"
)

func TestGoalContextBlock(t *testing.T) {
	// Active goal renders; empty or done renders nothing (stops steering).
	if b := GoalContextBlock("Ship v1", false); !strings.Contains(b, "Ship v1") || !strings.Contains(b, "north star") {
		t.Fatalf("active goal not rendered: %q", b)
	}
	if b := GoalContextBlock("", false); b != "" {
		t.Fatalf("empty goal should render nothing: %q", b)
	}
	if b := GoalContextBlock("Ship v1", true); b != "" {
		t.Fatalf("done goal should render nothing: %q", b)
	}
}

func TestAutonomousGoalBlock(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	r := &Runtime{db: database}
	ctx := context.Background()

	sess, err := database.CreateSession(ctx, db.Session{Kind: "chat", AgentID: "AGT1"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// No session in ctx → empty (safe no-op for unstamped turns).
	if b := r.autonomousGoalBlock(ctx); b != "" {
		t.Fatalf("no session id: want empty, got %q", b)
	}

	// Session in ctx but no goal → empty.
	sctx := WithSessionID(ctx, sess.ID)
	if b := r.autonomousGoalBlock(sctx); b != "" {
		t.Fatalf("no goal: want empty, got %q", b)
	}

	// Goal set → rendered. Done → empty again.
	if err := database.SetSessionGoal(ctx, sess.ID, "Reach the summit", false); err != nil {
		t.Fatalf("set goal: %v", err)
	}
	if b := r.autonomousGoalBlock(sctx); !strings.Contains(b, "Reach the summit") {
		t.Fatalf("goal not injected for autonomous turn: %q", b)
	}
	if err := database.SetSessionGoal(ctx, sess.ID, "Reach the summit", true); err != nil {
		t.Fatalf("complete goal: %v", err)
	}
	if b := r.autonomousGoalBlock(sctx); b != "" {
		t.Fatalf("completed goal should not steer: %q", b)
	}
}
