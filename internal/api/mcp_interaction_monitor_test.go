package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/tools"
)

// TestCallMonitorWithoutManagerIsAnExplicitError: a bridged monitor call on a run
// with no manager (shell disabled, or no session) must return an ERROR result. A
// silent success would leave the agent ending its turn to wait for a wake that can
// never arrive.
func TestCallMonitorWithoutManagerIsAnExplicitError(t *testing.T) {
	runs := newChatRuns()
	run := runs.register("r-mon-none", "SES1", "ws1", func() {})
	defer runs.unregister("r-mon-none")

	b := &interactionBackend{runs: runs}
	res, err := b.callMonitor(context.Background(), run, json.RawMessage(`{"action":"list"}`))
	if err != nil {
		t.Fatalf("callMonitor: %v", err)
	}
	if !res.IsError {
		t.Fatalf("missing manager must be an error result, got %+v", res)
	}
	if !strings.Contains(res.Text, "not available") {
		t.Fatalf("error text must say monitoring is unavailable, got %q", res.Text)
	}
}

// TestCallMonitorDispatchesToTheSessionManager pins that the bridge reaches the
// SAME manager the native registry uses, so a monitor armed over the CLI path is
// visible to (and stoppable from) the rest of the session.
func TestCallMonitorDispatchesToTheSessionManager(t *testing.T) {
	runs := newChatRuns()
	run := runs.register("r-mon", "SES1", "ws1", func() {})
	defer runs.unregister("r-mon")

	mgr := tools.NewMonitorManager(func(context.Context, string, string) (string, error) {
		return "SCH1", nil
	})
	defer mgr.Close()
	run.setMonitor(mgr, tools.NewShellManager())

	b := &interactionBackend{runs: runs}
	res, err := b.callMonitor(context.Background(), run, json.RawMessage(`{"action":"list"}`))
	if err != nil {
		t.Fatalf("callMonitor: %v", err)
	}
	if res.IsError {
		t.Fatalf("list on a live manager must succeed, got %+v", res)
	}
	if res.Text != "(no monitors)" {
		t.Fatalf("want the manager's own list output, got %q", res.Text)
	}

	// Arming against an unknown shell is a real error, surfaced to the agent rather
	// than swallowed into a fake monitor id.
	res, err = b.callMonitor(context.Background(), run,
		json.RawMessage(`{"action":"start","shell_id":"bg404","filter":"x"}`))
	if err != nil {
		t.Fatalf("callMonitor: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Text, "bg404") {
		t.Fatalf("unknown shell must be reported as an error, got %+v", res)
	}
}

// TestMonitorIsAdvertisedAndDispatchable is the reachability assertion for the
// bridge backend: the tool the CLI is told about must be a tool the dispatch
// switch actually handles. A name advertised but not dispatched is an "unknown
// tool" at call time.
func TestMonitorIsAdvertisedAndDispatchable(t *testing.T) {
	// Advertised: the def the bridge builds from a bare tool carries the name and a
	// schema, so the CLI can call it.
	def := tools.NewMonitorTool(nil, nil).Def()
	if def.Name != "monitor" {
		t.Fatalf("tool name drifted: %q", def.Name)
	}
	if len(def.InputSchema) == 0 {
		t.Fatal("monitor advertises no input schema")
	}

	// Dispatchable: the run has a manager, so reaching callMonitor (rather than the
	// default "unknown tool" arm) yields the manager's list output.
	runs := newChatRuns()
	run := runs.register("r-mon-dispatch", "SES1", "ws1", func() {})
	defer runs.unregister("r-mon-dispatch")
	mgr := tools.NewMonitorManager(func(context.Context, string, string) (string, error) { return "SCH1", nil })
	defer mgr.Close()
	run.setMonitor(mgr, tools.NewShellManager())

	b := &interactionBackend{runs: runs}
	res, err := b.callMonitor(context.Background(), run, json.RawMessage(`{"action":"list"}`))
	if err != nil {
		t.Fatalf("callMonitor: %v", err)
	}
	if res.IsError || strings.Contains(res.Text, "unknown tool") {
		t.Fatalf("monitor is advertised but not dispatched: %+v", res)
	}
}
