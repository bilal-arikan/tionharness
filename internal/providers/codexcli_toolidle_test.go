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

// TestCodexParserAnnouncesRunningTools: a started tool item is emitted once as
// a running step carrying the item id, and its completion reuses that id.
func TestCodexParserAnnouncesRunningTools(t *testing.T) {
	var got []TraceStep
	p := newCodexParser("gpt-test", func(st TraceStep) { got = append(got, st) })
	p.feed(`{"type":"item.started","item":{"id":"s1","type":"command_execution","command":"go test ./...","status":"in_progress"}}`)
	p.feed(`{"type":"item.updated","item":{"id":"s1","type":"command_execution","command":"go test ./...","status":"in_progress"}}`)
	p.feed(`{"type":"item.completed","item":{"id":"s1","type":"command_execution","command":"go test ./...","exit_code":0,"status":"completed"}}`)
	if len(got) != 2 || !got[0].Running || got[0].ID != "s1" || got[1].Running || got[1].ID != "s1" {
		t.Fatalf("want one running frame then the completed step, both id s1; got %+v", got)
	}
}

// TestCodexBridgeProbeWaitsForListener: the in-process bridge is probed until
// the listener comes up (a restart reclaims workers before it does); any other
// unreachable server is still dropped on the first failed probe.
func TestCodexBridgeProbeWaitsForListener(t *testing.T) {
	calls := map[string]int{}
	probe := func(_ context.Context, u string) bool {
		calls[u]++
		return u == "http://127.0.0.1:1/bridge" && calls[u] >= 3
	}
	servers := map[string]CLIMCPServer{
		"tionharness_interaction": {Transport: "http", URL: "http://127.0.0.1:1/bridge"},
		"remote":                  {Transport: "http", URL: "http://127.0.0.1:1/remote"},
	}
	kept, dropped := filterReachableCodexServers(context.Background(), servers, probe)
	if _, ok := kept["tionharness_interaction"]; !ok || calls["http://127.0.0.1:1/bridge"] != 3 {
		t.Fatalf("bridge must be retried until reachable: kept=%v calls=%v", kept, calls)
	}
	if len(dropped) != 1 || dropped[0] != "remote" || calls["http://127.0.0.1:1/remote"] != 1 {
		t.Fatalf("other servers keep the single probe: dropped=%v calls=%v", dropped, calls)
	}
}
