package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// TestStopSubagentIsWiredWithRunSubagent: the start and stop halves of async
// delegation are installed together, so a turn can never hold one without the
// other.
func TestStopSubagentIsWiredWithRunSubagent(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	ctx := context.Background()
	caller, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Caller", Provider: "anthropic", MCPEnabled: true})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if !rt.buildRegistry(ctx, caller).Has("stop_subagent") {
		t.Fatal("stop_subagent must be registered alongside run_subagent")
	}
	wired := rt.withRunAgent(context.Background(), caller, nil, false)
	if tools.StopAgentFrom(wired) == nil {
		t.Fatal("expected a stop-subagent runner in context")
	}
}

// TestStopSubagentRefusesForeignSessions: session ids are guessable, so the
// ownership check is what keeps one branch of the tree from cancelling another.
func TestStopSubagentRefusesForeignSessions(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	ctx := context.Background()
	a, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "Helper", Provider: "anthropic"})

	t.Run("not yours", func(t *testing.T) {
		_, childID := subagentChild(t, rt, a.ID, runStateRunning)
		other, err := rt.db.CreateSession(ctx, db.Session{AgentID: a.ID, Kind: "chat"})
		if err != nil {
			t.Fatalf("create other session: %v", err)
		}
		if _, err := rt.StopSubagent(ctx, other.ID, childID); err == nil ||
			!strings.Contains(err.Error(), "not one of your subagent runs") {
			t.Fatalf("expected ownership refusal, got %v", err)
		}
	})

	t.Run("not a subagent", func(t *testing.T) {
		plain, err := rt.db.CreateSession(ctx, db.Session{AgentID: a.ID, Kind: "chat"})
		if err != nil {
			t.Fatalf("create plain session: %v", err)
		}
		if _, err := rt.StopSubagent(ctx, plain.ID, plain.ID); err == nil ||
			!strings.Contains(err.Error(), "not a subagent run") {
			t.Fatalf("expected kind refusal, got %v", err)
		}
	})

	t.Run("blank id", func(t *testing.T) {
		if _, err := rt.StopSubagent(ctx, "SES1", "  "); err == nil {
			t.Fatal("expected a blank session id to be refused")
		}
	})
}

// TestStopSubagentOnFinishedRunIsNotAnError: by the time the caller decides to
// stop a detached run, it finishing first is a race the caller cannot win —
// reporting that as an error would push the model into pointless retries.
func TestStopSubagentOnFinishedRunIsNotAnError(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	ctx := context.Background()
	a, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "Helper", Provider: "anthropic"})
	parentID, childID := subagentChild(t, rt, a.ID, "completed")

	stopped, err := rt.StopSubagent(ctx, parentID, childID)
	if err != nil {
		t.Fatalf("stopping a finished run must not error: %v", err)
	}
	if stopped {
		t.Fatal("a finished run reports stopped=false")
	}
}

// TestStopSubagentCancelsAndMarksKilled: an in-flight run is cancelled and its
// terminal state is stamped before the call returns, so a caller that
// immediately re-reads the session never sees it still running.
func TestStopSubagentCancelsAndMarksKilled(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	ctx := context.Background()
	a, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "Helper", Provider: "anthropic"})
	parentID, childID := subagentChild(t, rt, a.ID, runStateRunning)

	cancelled := make(chan struct{})
	rt.trackSession(childID, func() { close(cancelled) })

	stopped, err := rt.StopSubagent(ctx, parentID, childID)
	if err != nil {
		t.Fatalf("stop subagent: %v", err)
	}
	if !stopped {
		t.Fatal("expected the in-flight run to report stopped=true")
	}
	select {
	case <-cancelled:
	default:
		t.Fatal("the child's turn context was not cancelled")
	}
	got, err := rt.db.GetSession(ctx, childID)
	if err != nil {
		t.Fatalf("re-read child: %v", err)
	}
	if got.RunState != "killed" {
		t.Fatalf("expected runState killed, got %q", got.RunState)
	}
}
