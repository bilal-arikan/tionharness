package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// The stamping assertion that used to live here moved to
// TestCallStampsSessionIDCentrally (mcp_interaction_stamp_test.go): the session id
// is now stamped once in Call(), so a direct callRunSubagent call no longer stamps
// anything and could not prove the behaviour.

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
