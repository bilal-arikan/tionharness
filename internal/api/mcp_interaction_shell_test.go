package api

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/agent"
)

// The stamping assertion that used to live here moved to
// TestCallStampsShellSessionID (mcp_interaction_stamp_test.go): the session id is
// now stamped once in Call(), so it has to be exercised through Call().

// TestCallShellWithoutSessionID: unlike a subagent, a shell call on a run with no
// persisted session is still legitimate — it must reach the runner rather than
// fail — but it falls back to the workspace default dir, which callShell warns about.
func TestCallShellWithoutSessionID(t *testing.T) {
	runs := newChatRuns()
	run := runs.register("r-shell-nosess", "", "ws1", func() {})
	defer runs.unregister("r-shell-nosess")

	called := false
	run.setShellRunner(func(ctx context.Context, _ string, _ json.RawMessage) (string, error) {
		called = true
		if got := agent.SessionIDFrom(ctx); got != "" {
			t.Errorf("agent.SessionIDFrom(ctx) = %q, want empty", got)
		}
		return "ok", nil
	})

	b := &interactionBackend{runs: runs}
	res, err := b.callShell(context.Background(), run, "Bash", json.RawMessage(`{"command":"echo hi"}`))
	if err != nil {
		t.Fatalf("callShell: %v", err)
	}
	if res.IsError || res.Text != "ok" {
		t.Fatalf("want the runner result, got %+v", res)
	}
	if !called {
		t.Error("runner must still be invoked when the run has no session id")
	}
}
