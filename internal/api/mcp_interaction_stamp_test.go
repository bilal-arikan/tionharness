package api

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// newStampRun registers a run reachable through Call() by its stable Interaction
// Bearer token, the way the CLI paths bind it.
func newStampRun(t *testing.T, runID, sessionID string) (*chatRuns, *chatRun, string) {
	t.Helper()
	runs := newChatRuns()
	run := runs.register(runID, sessionID, "ws1", func() {})
	t.Cleanup(func() { runs.unregister(runID) })
	tok := runs.interactionToken("ws1", sessionID, "AG1")
	runs.bindActive(tok, run)
	return runs, run, tok
}

// TestCallStampsSessionIDCentrally locks the single stamping point: Call() puts
// the run's session id on the ctx every bridged handler receives (stampRunSession),
// so a handler no longer has to remember to stamp it itself. Exercised through
// run_subagent, which needs BOTH keys — tools.CurrentSessionID for session-scoped
// built-ins and agent.SessionIDFrom for subagent parenting.
func TestCallStampsSessionIDCentrally(t *testing.T) {
	runs, run, tok := newStampRun(t, "r-stamp", "s-stamp")

	var gotAgentSession, gotToolSession string
	run.setRunAgent(func(ctx context.Context, _ json.RawMessage) (string, error) {
		gotAgentSession = agent.SessionIDFrom(ctx)
		gotToolSession = tools.CurrentSessionID(ctx)
		return "subagent done", nil
	})

	b := &interactionBackend{runs: runs}
	res, err := b.Call(context.Background(), tok, "run_subagent", json.RawMessage(`{"agent":"Coder","task":"x"}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
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

// TestCallStampsShellSessionID: the shell runner resolves the command's working
// directory from the session id on the context (Runtime.effectiveWorkDir). The
// Interaction HTTP server's request ctx carries none, so an unstamped call
// silently ran in the workspace default dir instead of the session's WorkingDir.
// Same central stamp, a second dispatch branch.
func TestCallStampsShellSessionID(t *testing.T) {
	shellName := ""
	for _, name := range tools.ShellToolNames() {
		shellName = name
		break
	}
	if shellName == "" {
		t.Skip("no shell interpreter is registered on this host")
	}
	runs, run, tok := newStampRun(t, "r-stamp-shell", "s-stamp-shell")

	var gotAgentSession, gotToolSession, gotTool string
	run.setShellRunner(func(ctx context.Context, toolName string, _ json.RawMessage) (string, error) {
		gotAgentSession = agent.SessionIDFrom(ctx)
		gotToolSession = tools.CurrentSessionID(ctx)
		gotTool = toolName
		return "ok", nil
	})

	tun := agent.NewTunables()
	tun.SetShellEnabled(true) // else interactionToolSpecs omits the shell and Call() reports "No such tool"
	b := &interactionBackend{runs: runs, tun: tun}
	res, err := b.Call(context.Background(), tok, shellName, json.RawMessage(`{"command":"echo hi"}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if res.IsError || res.Text != "ok" {
		t.Fatalf("want the runner result, got %+v", res)
	}
	if gotTool != shellName {
		t.Errorf("toolName = %q, want %q", gotTool, shellName)
	}
	if gotAgentSession != run.sessionID {
		t.Errorf("agent.SessionIDFrom(ctx) = %q, want %q", gotAgentSession, run.sessionID)
	}
	if gotToolSession != run.sessionID {
		t.Errorf("tools.CurrentSessionID(ctx) = %q, want %q", gotToolSession, run.sessionID)
	}
}

// ctxCaptureTool is a tools.Tool that only records the ctx it was called with.
type ctxCaptureTool struct{ ctx context.Context }

func (c *ctxCaptureTool) Def() providers.ToolDef { return providers.ToolDef{Name: "capture"} }

func (c *ctxCaptureTool) Call(ctx context.Context, _ json.RawMessage) (string, error) {
	c.ctx = ctx
	return "ok", nil
}

// TestCallViaSinkRestampsAfterAttach: the artifact/notify/focus_view/todo_write
// attach closures deliberately swap in a fresh context.Background() so the write
// survives turn cancellation ("Durdur"). That swap also dropped the session stamp
// Call() had just applied, silently detaching the sink-bound tool from its
// session. callViaSink must re-apply the stamp on whatever ctx attach returned.
func TestCallViaSinkRestampsAfterAttach(t *testing.T) {
	runs, run, _ := newStampRun(t, "r-stamp-sink", "s-stamp-sink")
	_ = runs

	capture := &ctxCaptureTool{}
	st := sinkTool{
		newTool: func() tools.Tool { return capture },
		// Exactly what artifactAttach/notify/focus_view do: discard the call ctx.
		attach: func(_ context.Context, _ *chatRun) (context.Context, bool) {
			return context.Background(), true
		},
	}

	b := &interactionBackend{runs: runs}
	res, err := b.callViaSink(stampRunSession(context.Background(), run), run, st, nil)
	if err != nil {
		t.Fatalf("callViaSink: %v", err)
	}
	if res.IsError || res.Text != "ok" {
		t.Fatalf("want the tool result, got %+v", res)
	}
	if capture.ctx == nil {
		t.Fatal("tool was never called")
	}
	if got := agent.SessionIDFrom(capture.ctx); got != run.sessionID {
		t.Errorf("agent.SessionIDFrom(ctx) = %q, want %q", got, run.sessionID)
	}
	if got := tools.CurrentSessionID(capture.ctx); got != run.sessionID {
		t.Errorf("tools.CurrentSessionID(ctx) = %q, want %q", got, run.sessionID)
	}
}

// TestStampRunSessionEmptyIsNoOp: the empty case must stay a no-op, so each tool
// keeps deciding what "no session" means (callRunSubagent rejects the call,
// callShell warns and runs in the workspace default dir). Stamping an empty id
// would make those checks read a present-but-empty value instead.
func TestStampRunSessionEmptyIsNoOp(t *testing.T) {
	runs := newChatRuns()
	run := runs.register("r-stamp-empty", "", "ws1", func() {})
	defer runs.unregister("r-stamp-empty")

	ctx := stampRunSession(context.Background(), run)
	if got := agent.SessionIDFrom(ctx); got != "" {
		t.Errorf("agent.SessionIDFrom(ctx) = %q, want empty", got)
	}
	if got := tools.CurrentSessionID(ctx); got != "" {
		t.Errorf("tools.CurrentSessionID(ctx) = %q, want empty", got)
	}
	if stampRunSession(context.Background(), nil) == nil {
		t.Error("stampRunSession(ctx, nil) must return the ctx, not nil")
	}
}
