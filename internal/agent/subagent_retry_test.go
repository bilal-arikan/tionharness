package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// subagentChild creates a parent chat session plus one subagent child of it, in
// the given run state, and returns both ids.
func subagentChild(t *testing.T, rt *Runtime, agentID, runState string) (parentID, childID string) {
	t.Helper()
	ctx := context.Background()
	parent, err := rt.db.CreateSession(ctx, db.Session{AgentID: agentID, Kind: "chat"})
	if err != nil {
		t.Fatalf("create parent session: %v", err)
	}
	child, err := rt.db.CreateChildSession(ctx, db.Session{
		AgentID:         agentID,
		Kind:            subagentSessionKind,
		ParentSessionID: parent.ID,
		ExecutionType:   db.ExecutionSubagent,
		Category:        db.CategorySubagent,
		ContextMode:     db.ContextIsolated,
		Visibility:      db.VisibilityInternal,
		TargetAgentID:   agentID,
	})
	if err != nil {
		t.Fatalf("create child session: %v", err)
	}
	if runState != "" {
		if err := rt.db.SetSessionRunState(ctx, child.ID, runState, time.Now().Unix()); err != nil {
			t.Fatalf("set run state: %v", err)
		}
	}
	return parent.ID, child.ID
}

// TestResolveRetryLineageNumbersAttempts: a retry links to the attempt it
// replaces and counts up from it, so the chain is walkable and a single row says
// which try it is.
func TestResolveRetryLineageNumbersAttempts(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	ctx := context.Background()
	a, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "Helper", Provider: "anthropic"})
	parentID, firstID := subagentChild(t, rt, a.ID, "failed")

	prev, attempt, err := rt.resolveRetryLineage(ctx, parentID, firstID)
	if err != nil {
		t.Fatalf("resolve retry lineage: %v", err)
	}
	if prev != firstID {
		t.Fatalf("retry should link to the previous attempt, got %q", prev)
	}
	// A pre-lineage row carries no attempt number; it is the first try by definition.
	if attempt != 2 {
		t.Fatalf("expected attempt 2, got %d", attempt)
	}

	// A retry OF a retry keeps counting rather than resetting.
	second, err := rt.db.CreateChildSession(ctx, db.Session{
		AgentID: a.ID, Kind: subagentSessionKind, ParentSessionID: parentID,
		ExecutionType: db.ExecutionSubagent, Category: db.CategorySubagent,
		ContextMode: db.ContextIsolated, Visibility: db.VisibilityInternal,
		TargetAgentID: a.ID, RetryOfSessionID: firstID, Attempt: attempt,
	})
	if err != nil {
		t.Fatalf("create second attempt: %v", err)
	}
	if err := rt.db.SetSessionRunState(ctx, second.ID, "failed", time.Now().Unix()); err != nil {
		t.Fatalf("set run state: %v", err)
	}
	prev, attempt, err = rt.resolveRetryLineage(ctx, parentID, second.ID)
	if err != nil {
		t.Fatalf("resolve second retry: %v", err)
	}
	if prev != second.ID || attempt != 3 {
		t.Fatalf("expected link to %s at attempt 3, got %s at %d", second.ID, prev, attempt)
	}
}

// TestResolveRetryLineageEmptyIsNotARetry: the field is optional, and an absent
// one must leave the run unstamped rather than inventing attempt 1.
func TestResolveRetryLineageEmptyIsNotARetry(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	prev, attempt, err := rt.resolveRetryLineage(context.Background(), "SES1", "  ")
	if err != nil || prev != "" || attempt != 0 {
		t.Fatalf("empty retry_of should be a no-op, got (%q, %d, %v)", prev, attempt, err)
	}
}

// TestResolveRetryLineageRejectsForeignAndLiveRuns covers the two refusals that
// protect the tree: a run belonging to someone else, and one still in flight.
func TestResolveRetryLineageRejectsForeignAndLiveRuns(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	ctx := context.Background()
	a, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "Helper", Provider: "anthropic"})

	t.Run("not yours", func(t *testing.T) {
		_, childID := subagentChild(t, rt, a.ID, "failed")
		otherParent, err := rt.db.CreateSession(ctx, db.Session{AgentID: a.ID, Kind: "chat"})
		if err != nil {
			t.Fatalf("create other session: %v", err)
		}
		_, _, err = rt.resolveRetryLineage(ctx, otherParent.ID, childID)
		if err == nil || !strings.Contains(err.Error(), "not one of your subagent runs") {
			t.Fatalf("expected ownership refusal, got %v", err)
		}
	})

	t.Run("still running", func(t *testing.T) {
		parentID, childID := subagentChild(t, rt, a.ID, runStateRunning)
		_, _, err := rt.resolveRetryLineage(ctx, parentID, childID)
		if err == nil || !strings.Contains(err.Error(), "still running") {
			t.Fatalf("expected live-run refusal, got %v", err)
		}
	})

	t.Run("not a subagent run", func(t *testing.T) {
		plain, err := rt.db.CreateSession(ctx, db.Session{AgentID: a.ID, Kind: "chat"})
		if err != nil {
			t.Fatalf("create plain session: %v", err)
		}
		_, _, err = rt.resolveRetryLineage(ctx, plain.ID, plain.ID)
		if err == nil || !strings.Contains(err.Error(), "not a subagent run") {
			t.Fatalf("expected kind refusal, got %v", err)
		}
	})

	t.Run("missing", func(t *testing.T) {
		_, _, err := rt.resolveRetryLineage(ctx, "SES1", "SES-nope")
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("expected not-found refusal, got %v", err)
		}
	})
}

// TestStampRetryLineageLeavesPlainRunsAlone: a non-retry child must not gain the
// fields at all, so "part of a retry chain" stays a single readable condition.
func TestStampRetryLineageLeavesPlainRunsAlone(t *testing.T) {
	var meta db.Session
	stampRetryLineage(&meta, "", 0)
	if meta.RetryOfSessionID != "" || meta.Attempt != 0 {
		t.Fatalf("plain run must stay unstamped, got %+v", meta)
	}
	stampRetryLineage(&meta, "SES9", 2)
	if meta.RetryOfSessionID != "SES9" || meta.Attempt != 2 {
		t.Fatalf("retry run should carry the lineage, got %+v", meta)
	}
}
