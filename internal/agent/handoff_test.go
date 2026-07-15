package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/prompts"
)

// TestHandoffChainDepth walks the ParentSessionID lineage and counts resets.
func TestHandoffChainDepth(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()

	root, err := rt.db.CreateSession(ctx, db.Session{Kind: "chat"})
	if err != nil {
		t.Fatalf("create root: %v", err)
	}
	child, err := rt.db.CreateSession(ctx, db.Session{Kind: "spawned", ParentSessionID: root.ID})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}
	grand, err := rt.db.CreateSession(ctx, db.Session{Kind: "spawned", ParentSessionID: child.ID})
	if err != nil {
		t.Fatalf("create grand: %v", err)
	}

	if d := rt.handoffChainDepth(ctx, root); d != 0 {
		t.Errorf("root depth = %d, want 0", d)
	}
	if d := rt.handoffChainDepth(ctx, child); d != 1 {
		t.Errorf("child depth = %d, want 1", d)
	}
	if d := rt.handoffChainDepth(ctx, grand); d != 2 {
		t.Errorf("grand depth = %d, want 2", d)
	}
}

// TestMaybeAutoHandoff_NoOps verifies the guards: nothing spawns unless the turn
// overflowed AND auto-handoff is enabled.
func TestMaybeAutoHandoff_NoOps(t *testing.T) {
	rt, tun := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	agent, err := rt.db.CreateAgent(ctx, db.Agent{Name: "W", Provider: "anthropic", Model: "m"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sess, err := rt.db.CreateSession(ctx, db.Session{AgentID: agent.ID, Kind: "schedule"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	count := func() int {
		ss, _ := rt.db.ListSessions(ctx, "")
		return len(ss)
	}
	before := count()

	// Auto off (default) → no-op even when overflowed.
	rt.maybeAutoHandoff(ctx, sess.ID, agent, true)
	if count() != before {
		t.Errorf("handoff happened with HandoffAuto off")
	}

	// Auto on but not overflowed → no-op.
	tun.SetHandoff(true, 0, 0, false)
	rt.maybeAutoHandoff(ctx, sess.ID, agent, false)
	if count() != before {
		t.Errorf("handoff happened without an overflow")
	}
}

// TestMaybeAutoHandoff_ChainCap stops auto-reset once the lineage hits the cap.
func TestMaybeAutoHandoff_ChainCap(t *testing.T) {
	rt, tun := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	tun.SetHandoff(true, 0, 1, false) // cap = 1
	ctx := context.Background()
	agent, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "W", Provider: "anthropic", Model: "m"})

	root, _ := rt.db.CreateSession(ctx, db.Session{AgentID: agent.ID, Kind: "chat"})
	child, _ := rt.db.CreateSession(ctx, db.Session{AgentID: agent.ID, Kind: "spawned", ParentSessionID: root.ID})

	ss, _ := rt.db.ListSessions(ctx, "")
	before := len(ss)
	// child is already at depth 1 == cap → must not reset (no provider call attempted).
	rt.maybeAutoHandoff(ctx, child.ID, agent, true)
	ss, _ = rt.db.ListSessions(ctx, "")
	if len(ss) != before {
		t.Errorf("handoff happened past the chain cap")
	}
}

// TestBuildContinuationPrompt embeds the handoff inline and the recovery pointers.
func TestBuildContinuationPrompt(t *testing.T) {
	got := buildContinuationPrompt(prompts.Default("continuation"), "SES7", "ART3", "/w/.tionswarm/handoff.md", "1. Objective: do X")
	for _, want := range []string{"FRESH context window", "SES7", "ART3", "conversation_search", "do X", "/w/.tionswarm/handoff.md"} {
		if !strings.Contains(got, want) {
			t.Errorf("continuation prompt missing %q", want)
		}
	}
}

// TestHandoffTitle strips a leading reset marker so chained handoffs don't accrete.
func TestHandoffTitle(t *testing.T) {
	if got := handoffTitle(db.Session{Title: "↪ Build parser"}); got != "Build parser" {
		t.Errorf("title = %q, want %q", got, "Build parser")
	}
	if got := handoffTitle(db.Session{ID: "SES1", Title: "   "}); got != "SES1" {
		t.Errorf("blank title should fall back to id, got %q", got)
	}
}
