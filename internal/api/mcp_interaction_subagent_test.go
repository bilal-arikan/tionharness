package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// TestCallRunSubagentStampsSessionID locks the fix for the bridged run_subagent
// path (claude-cli / codex-cli): the runner used to execute on the Interaction
// HTTP server's request context, which carries no session id, so runAgent's
// agent.SessionIDFrom(ctx) came back empty and EVERY bridged call died with
// "subagent persistence requires a parent session". callRunSubagent must stamp
// the run's session id on the context it hands to the runner.
func TestCallRunSubagentStampsSessionID(t *testing.T) {
	runs := newChatRuns()
	run := runs.register("r-sub", "s-sub", "ws1", func() {})
	defer runs.unregister("r-sub")

	var gotAgentSession, gotToolSession string
	run.setRunAgent(func(ctx context.Context, _ json.RawMessage) (string, error) {
		gotAgentSession = agent.SessionIDFrom(ctx)
		gotToolSession = tools.CurrentSessionID(ctx)
		return "subagent done", nil
	})

	b := &interactionBackend{runs: runs}
	res, err := b.callRunSubagent(context.Background(), run, json.RawMessage(`{"agent":"Coder","task":"x"}`))
	if err != nil {
		t.Fatalf("callRunSubagent: %v", err)
	}
	if res.IsError || res.Text != "subagent done" {
		t.Fatalf("want the runner result, got %+v", res)
	}
	if gotAgentSession != run.sessionID {
		t.Errorf("agent.SessionIDFrom(ctx) = %q, want %q", gotAgentSession, run.sessionID)
	}
	if gotToolSession != run.sessionID {
		t.Errorf("tools.CurrentSessionID(ctx) = %q, want %q", gotToolSession, run.sessionID)
	}
}

// TestCallRunSubagentWithoutSessionID: a turn with no persisted session cannot
// parent a subagent, so the call must fail loudly before the runner is invoked
// instead of reaching runAgent and failing there on an empty session id.
func TestCallRunSubagentWithoutSessionID(t *testing.T) {
	runs := newChatRuns()
	run := runs.register("r-nosess", "", "ws1", func() {})
	defer runs.unregister("r-nosess")

	called := false
	run.setRunAgent(func(ctx context.Context, _ json.RawMessage) (string, error) {
		called = true
		return "should not run", nil
	})

	b := &interactionBackend{runs: runs}
	res, err := b.callRunSubagent(context.Background(), run, json.RawMessage(`{"agent":"Coder","task":"x"}`))
	if err != nil {
		t.Fatalf("callRunSubagent: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Text, "persisted session") {
		t.Fatalf("want an error result mentioning a persisted session, got %+v", res)
	}
	if called {
		t.Error("runner must not be invoked when the run has no session id")
	}
}
