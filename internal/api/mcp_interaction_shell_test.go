package api

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// TestCallShellStampsSessionID locks the fix for the bridged Bash/PowerShell path:
// the shell runner resolves the command's working directory from the session id on
// the context (Runtime.effectiveWorkDir). The Interaction HTTP server's request ctx
// carries none, so an unstamped call silently ran in the workspace default dir
// instead of the session's WorkingDir — no error, no warning.
func TestCallShellStampsSessionID(t *testing.T) {
	runs := newChatRuns()
	run := runs.register("r-shell", "s-shell", "ws1", func() {})
	defer runs.unregister("r-shell")

	var gotAgentSession, gotToolSession, gotTool string
	run.setShellRunner(func(ctx context.Context, toolName string, _ json.RawMessage) (string, error) {
		gotAgentSession = agent.SessionIDFrom(ctx)
		gotToolSession = tools.CurrentSessionID(ctx)
		gotTool = toolName
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
	if gotTool != "Bash" {
		t.Errorf("toolName = %q, want %q", gotTool, "Bash")
	}
	if gotAgentSession != run.sessionID {
		t.Errorf("agent.SessionIDFrom(ctx) = %q, want %q", gotAgentSession, run.sessionID)
	}
	if gotToolSession != run.sessionID {
		t.Errorf("tools.CurrentSessionID(ctx) = %q, want %q", gotToolSession, run.sessionID)
	}
}

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
