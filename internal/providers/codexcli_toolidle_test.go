package providers

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

// Regression (SES287 → SES294): codex called the bridged run_subagent and went
// silent while the child worked. The stdout watchdog read that as a hang after
// one window, killed codex and, through the aborted bridge request, the child.
// While a tool item is open the watchdog must leave the turn to its context.
func TestCodexIdleWatchdogWaitsOutRunningTool(t *testing.T) {
	if os.Getenv("CODEX_TEST_HELPER") != "" {
		return
	}
	const window = 100 * time.Millisecond
	self, err := os.Executable()
	if err != nil {
		t.Skipf("no test binary path: %v", err)
	}
	t.Setenv("CODEX_TEST_STALL_LINE",
		`{"type":"item.started","item":{"id":"m1","type":"mcp_tool_call","server":"tionharness","tool":"run_subagent","status":"in_progress"}}`)
	original := codexIdleOutputWindow()
	SetCodexIdleOutputTimeout(window)
	t.Cleanup(func() { SetCodexIdleOutputTimeout(original) })

	// The caller's context (the turn watchdog in production) is what ends it.
	ctx, cancel := context.WithTimeout(context.Background(), 6*window)
	defer cancel()
	c := &CodexCLI{binPath: self}
	start := time.Now()
	_, _, runErr := c.runAttempt(ctx, []string{"-test.run=TestCodexHelperEmitsThenStalls", "-test.v=false"}, "prompt", "gpt-test", Request{}, "")
	elapsed := time.Since(start)

	if isCodexIdleHang(runErr) {
		t.Fatalf("stdout watchdog killed a turn whose tool was still running: %v", runErr)
	}
	if elapsed < 6*window-20*time.Millisecond {
		t.Fatalf("returned after %s, before the caller's deadline %s", elapsed, 6*window)
	}
	var interruption *CLIInterruption
	if errors.As(runErr, &interruption) && interruption.Reason == "cli_idle_timeout" {
		t.Fatalf("interruption reported as an idle timeout: %+v", interruption)
	}
}

// Once the tool reports completion the silence is codex's own again, so the
// ordinary one-window watchdog applies.
func TestCodexIdleWatchdogResumesAfterToolCompletes(t *testing.T) {
	if os.Getenv("CODEX_TEST_HELPER") != "" {
		return
	}
	const window = 150 * time.Millisecond
	elapsed, _, err := runCodexStallHelper(t, window,
		`{"type":"item.started","item":{"id":"m1","type":"mcp_tool_call","server":"tionharness","tool":"run_subagent","status":"in_progress"}}`+"\n"+
			`{"type":"item.completed","item":{"id":"m1","type":"mcp_tool_call","server":"tionharness","tool":"run_subagent","status":"completed"}}`)
	if !isCodexIdleHang(err) {
		t.Fatalf("expected the idle watchdog to fire after the tool completed, got %v", err)
	}
	if elapsed > 4*window {
		t.Fatalf("killed after %s, want about one window (%s)", elapsed, window)
	}
}

func TestCodexParserTracksToolItems(t *testing.T) {
	p := newCodexParser("gpt-test", nil)
	p.feed(`{"type":"item.started","item":{"id":"s1","type":"command_execution","command":"sleep 100","status":"in_progress"}}`)
	if !p.toolInFlight() {
		t.Fatal("a started command_execution is not in flight")
	}
	p.feed(`{"type":"item.started","item":{"id":"r1","type":"reasoning","text":"…"}}`)
	p.feed(`{"type":"item.completed","item":{"id":"s1","type":"command_execution","command":"sleep 100","exit_code":0,"status":"completed"}}`)
	if p.toolInFlight() {
		t.Fatal("a completed command_execution is still in flight")
	}
}

func TestOpenCodeParserTracksToolParts(t *testing.T) {
	p := newOpenCodeParser("m", nil, nil)
	p.feed(`{"type":"tool_use","part":{"type":"tool","tool":"bash","callID":"c1","state":{"status":"running","input":{}}}}`)
	if !p.toolInFlight() {
		t.Fatal("a running tool part is not in flight")
	}
	p.feed(`{"type":"tool_use","part":{"type":"tool","tool":"bash","callID":"c1","state":{"status":"completed","input":{},"output":"ok"}}}`)
	if p.toolInFlight() {
		t.Fatal("a completed tool part is still in flight")
	}
}
