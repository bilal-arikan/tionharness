package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// TestTurnCtxWrapsSurviveLaterPhases pins the invariant that makes the
// completeTracedInner phase split safe: ctx is a FIELD of toolLoopTurn, so every
// wrap a phase installs is observable by every phase that runs after it. Demoting
// any `t.ctx = with...(t.ctx, ...)` to a local variable confines that wrap to its
// own phase and silently drops the value from the rest of the turn — which this
// test turns into a failure.
func TestTurnCtxWrapsSurviveLaterPhases(t *testing.T) {
	rt := loopRuntime(t)
	if err := rt.db.SetWorkspaceToolConfig(context.Background(), db.WorkspaceToolConfig{
		ToolVisibility: map[string]string{"get_settings": tools.VisibilitySummary},
	}); err != nil {
		t.Fatalf("set workspace tool config: %v", err)
	}

	turn := toolLoopTurn{
		r:        rt,
		ctx:      WithSessionID(context.Background(), "s1"),
		agent:    db.Agent{ID: "a1", Model: "m", MCPEnabled: true},
		provider: &fakeProvider{},
	}

	cleanup, err := turn.prepare()
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	defer cleanup()

	// prepare's own wraps, read back from the turn's ctx.
	wd, ok := resolvedWorkDirFromCtx(turn.ctx)
	if !ok || wd.dir == "" {
		t.Fatalf("resolved work dir after prepare = (%+v, %v), want a directory", wd, ok)
	}
	if got := tools.CurrentSessionID(turn.ctx); got != "s1" {
		t.Fatalf("current session after prepare = %q, want %q", got, "s1")
	}
	if !tools.HasArtifactSink(turn.ctx) {
		t.Fatal("artifact sink missing from turn ctx after prepare")
	}
	if !tools.HasTodoSink(turn.ctx) {
		t.Fatal("todo sink missing from turn ctx after prepare")
	}

	resp, steps, err, done := turn.prepareNativeLoop()
	if err != nil {
		t.Fatalf("prepareNativeLoop: %v", err)
	}
	if done {
		t.Fatalf("prepareNativeLoop finished the turn early (resp=%v, steps=%v)", resp, steps)
	}

	// prepareNativeLoop's own wrap: the active set the loop mutates must be the
	// one reachable from ctx, since buildRegistry wires the activate_tools
	// meta-tools through activeToolsFromCtx.
	if turn.active == nil {
		t.Fatal("prepareNativeLoop left turn.active nil")
	}
	if got := activeToolsFromCtx(turn.ctx); got != turn.active {
		t.Fatalf("active tools from ctx = %p, want the turn's own set %p", got, turn.active)
	}

	// The chain, not just the last link: prepare's wraps must still be readable
	// after a later phase rebound ctx on top of them.
	if wd2, ok := resolvedWorkDirFromCtx(turn.ctx); !ok || wd2 != wd {
		t.Fatalf("resolved work dir after prepareNativeLoop = (%+v, %v), want %+v", wd2, ok, wd)
	}
	if got := tools.CurrentSessionID(turn.ctx); got != "s1" {
		t.Fatalf("current session after prepareNativeLoop = %q, want %q", got, "s1")
	}
	if !tools.HasArtifactSink(turn.ctx) {
		t.Fatal("artifact sink lost from turn ctx by prepareNativeLoop")
	}
	if !tools.HasTodoSink(turn.ctx) {
		t.Fatal("todo sink lost from turn ctx by prepareNativeLoop")
	}
}

// TestLoop_ActivateToolsUsesTurnActiveSetFromContext is the behavioural half of
// the invariant above: the active set travels from prepareNativeLoop to
// buildRegistry through ctx alone, so an explicit activate_tools call only has an
// effect when that wrap reaches the registry. The auto-activation path cannot
// cover this — it takes the active set as a direct argument
// (ConfigureAutoActivation), not from ctx.
func TestLoop_ActivateToolsUsesTurnActiveSetFromContext(t *testing.T) {
	rt := loopRuntime(t)
	rt.SetSettingsBridge(&countingSettingsBridge{})
	if err := rt.db.SetWorkspaceToolConfig(context.Background(), db.WorkspaceToolConfig{
		ToolVisibility: map[string]string{"get_settings": tools.VisibilitySummary},
	}); err != nil {
		t.Fatalf("set workspace tool config: %v", err)
	}

	agent := db.Agent{ID: "a1", Model: "m", MCPEnabled: true}
	fp := &fakeProvider{script: []scriptedResp{
		{stop: providers.StopToolUse, toolCalls: []providers.ToolCall{{
			ID:    "act-1",
			Name:  "activate_tools",
			Input: json.RawMessage(`{"names":["get_settings"]}`),
		}}},
		{stop: providers.StopEndTurn, text: "ready"},
	}}

	req := providers.Request{Messages: []providers.Message{{Role: providers.RoleUser, Text: "load settings tool"}}}
	if _, _, err := rt.CompleteWithToolsTraced(context.Background(), agent, fp, req, false); err != nil {
		t.Fatalf("complete with tools: %v", err)
	}
	if fp.calls != 2 {
		t.Fatalf("provider calls = %d, want 2", fp.calls)
	}
	if requestHasTool(fp.requests[0], "get_settings") {
		t.Fatal("lazy tool schema shipped before activation")
	}
	res, ok := lastToolResult(fp.requests[1])
	if !ok || res.IsError || !strings.Contains(res.Content, "Activated") {
		t.Fatalf("activate_tools result = %#v, want an activation confirmation", res)
	}
	if !requestHasTool(fp.requests[1], "get_settings") {
		t.Fatal("activated lazy tool schema absent from the next provider request")
	}
}
