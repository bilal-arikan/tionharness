package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// scriptedResp is one programmed reply from fakeProvider.
type scriptedResp struct {
	text      string
	stop      string
	toolCalls []providers.ToolCall
	err       error
}

// fakeProvider returns a fixed sequence of replies, letting a test drive the
// native tool loop through its recovery branches deterministically (no key, no
// network). Calls past the script return a plain end_turn.
type fakeProvider struct {
	calls     int
	script    []scriptedResp
	requests  []providers.Request
	onRequest func(int, providers.Request)
}

func (f *fakeProvider) Name() string { return "fake" }

func (f *fakeProvider) Complete(_ context.Context, req providers.Request) (*providers.Response, error) {
	f.requests = append(f.requests, req)
	i := f.calls
	f.calls++
	if f.onRequest != nil {
		f.onRequest(i, req)
	}
	if i >= len(f.script) {
		return &providers.Response{StopReason: providers.StopEndTurn, Text: ""}, nil
	}
	s := f.script[i]
	if s.err != nil {
		return nil, s.err
	}
	return &providers.Response{StopReason: s.stop, Text: s.text, ToolCalls: s.toolCalls}, nil
}

type countingSettingsBridge struct{ snapshots int }

func (*countingSettingsBridge) Path() string { return "settings.json" }

func (b *countingSettingsBridge) Snapshot() (string, error) {
	b.snapshots++
	return `{"theme":"dark"}`, nil
}

func (*countingSettingsBridge) Apply(string) (string, error) { return "", nil }

func requestHasTool(req providers.Request, name string) bool {
	for _, def := range req.Tools {
		if def.Name == name {
			return true
		}
	}
	return false
}

func lastToolResult(req providers.Request) (providers.ToolResult, bool) {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		results := req.Messages[i].ToolResults
		if len(results) > 0 {
			return results[len(results)-1], true
		}
	}
	return providers.ToolResult{}, false
}

// loopRuntime builds a runtime whose sandbox registers the built-in fs tools, so
// the agentic loop (which requires a non-empty registry) is actually entered.
func loopRuntime(t *testing.T) *Runtime {
	t.Helper()
	workDir := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	rt, _ := newTestRuntime(t, workDir)
	return rt
}

func hasRecovery(steps []TurnStep, reason string) bool {
	for _, s := range steps {
		if s.Kind == StepRecovery && s.Reason == reason {
			return true
		}
	}
	return false
}

// TestLoop_LazyToolAutoActivation drives the native structured-tool loop across
// the activation boundary. The first speculative call only activates the tool;
// the reissued call executes its handler after the schema reaches the provider.
func TestLoop_LazyToolAutoActivation(t *testing.T) {
	rt := loopRuntime(t)
	bridge := &countingSettingsBridge{}
	rt.SetSettingsBridge(bridge)
	if err := rt.db.SetWorkspaceToolConfig(context.Background(), db.WorkspaceToolConfig{
		ToolVisibility: map[string]string{"get_settings": tools.VisibilitySummary},
	}); err != nil {
		t.Fatalf("set workspace tool config: %v", err)
	}

	agent := db.Agent{ID: "a1", Model: "m", MCPEnabled: true}
	call := providers.ToolCall{ID: "settings-1", Name: "get_settings", Input: json.RawMessage(`{}`)}
	var snapshotsAtRequest []int
	fp := &fakeProvider{script: []scriptedResp{
		{stop: providers.StopToolUse, toolCalls: []providers.ToolCall{call}},
		{stop: providers.StopToolUse, toolCalls: []providers.ToolCall{{ID: "settings-2", Name: call.Name, Input: call.Input}}},
		{stop: providers.StopEndTurn, text: "settings loaded"},
	}, onRequest: func(_ int, _ providers.Request) {
		snapshotsAtRequest = append(snapshotsAtRequest, bridge.snapshots)
	}}

	req := providers.Request{Messages: []providers.Message{{Role: providers.RoleUser, Text: "read settings"}}}
	resp, _, err := rt.CompleteWithToolsTraced(context.Background(), agent, fp, req, false)
	if err != nil {
		t.Fatalf("complete with tools: %v", err)
	}
	if fp.calls != 3 {
		t.Fatalf("provider calls = %d, want 3", fp.calls)
	}
	if requestHasTool(fp.requests[0], call.Name) {
		t.Fatal("inactive lazy tool schema present in first provider request")
	}
	if !requestHasTool(fp.requests[1], call.Name) {
		t.Fatal("activated lazy tool schema absent from second provider request")
	}
	first, ok := lastToolResult(fp.requests[1])
	if !ok || !first.IsError || !strings.Contains(first.Content, "activated automatically") {
		t.Fatalf("first tool result = %#v, want auto-activation error", first)
	}
	second, ok := lastToolResult(fp.requests[2])
	if !ok || second.IsError || !strings.Contains(second.Content, `{"theme":"dark"}`) {
		t.Fatalf("second tool result = %#v, want successful settings snapshot", second)
	}
	if bridge.snapshots != 1 {
		t.Fatalf("settings handler calls = %d, want 1", bridge.snapshots)
	}
	if got := snapshotsAtRequest; len(got) != 3 || got[0] != 0 || got[1] != 0 || got[2] != 1 {
		t.Fatalf("settings handler calls at provider requests = %v, want [0 0 1]", got)
	}
	if resp.Text != "settings loaded" || resp.StopReason != providers.StopEndTurn {
		t.Fatalf("final response = (%q, %q), want final assistant text", resp.Text, resp.StopReason)
	}
}

// TestLoop_MaxTokenResume drives the full loop: the model is cut off by the
// output cap once, the loop injects a resume turn, and the capped fragments are
// stitched into the final answer — with a StepRecovery recorded.
func TestLoop_MaxTokenResume(t *testing.T) {
	rt := loopRuntime(t)
	agent := db.Agent{ID: "a1", Model: "m", MCPEnabled: true}
	fp := &fakeProvider{script: []scriptedResp{
		{stop: providers.StopMaxTok, text: "part one "},
		{stop: providers.StopEndTurn, text: "part two"},
	}}

	req := providers.Request{Messages: []providers.Message{{Role: providers.RoleUser, Text: "write a lot"}}}
	resp, steps, err := rt.CompleteWithToolsTraced(context.Background(), agent, fp, req, false)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if resp.Text != "part one part two" {
		t.Errorf("stitched text = %q, want %q", resp.Text, "part one part two")
	}
	if fp.calls != 2 {
		t.Errorf("provider calls = %d, want 2 (initial + resume)", fp.calls)
	}
	if !hasRecovery(steps, string(contMaxTokenResume)) {
		t.Errorf("no max_output_tokens_recovery StepRecovery in trace: %+v", steps)
	}
}

// TestLoop_ReactiveCompact drives the full loop through a context-overflow error:
// the loop folds the in-flight history once (a real CompactInFlightMessages call,
// which also goes through the provider) and retries to completion.
func TestLoop_ReactiveCompact(t *testing.T) {
	rt := loopRuntime(t)
	agent := db.Agent{ID: "a1", Model: "m", MCPEnabled: true}

	// Enough alternating turns that compaction finds an assistant fold boundary
	// at or before len-reactiveKeepRecent.
	var msgs []providers.Message
	for i := 0; i < 6; i++ {
		msgs = append(msgs, providers.Message{Role: providers.RoleUser, Text: "u"})
		msgs = append(msgs, providers.Message{Role: providers.RoleAssistant, Text: "a"})
	}
	msgs = append(msgs, providers.Message{Role: providers.RoleUser, Text: "final question"})

	fp := &fakeProvider{script: []scriptedResp{
		{err: errors.New("prompt is too long: 250000 tokens > 200000 maximum")}, // initial overflow
		{stop: providers.StopEndTurn, text: "SUMMARY"},                          // compaction's summarize call
		{stop: providers.StopEndTurn, text: "recovered answer"},                 // retry after compaction
	}}

	req := providers.Request{Messages: msgs, ResumeSessionID: "stale-pre-fold-thread", CLIResumeScope: "scope"}
	resp, steps, err := rt.CompleteWithToolsTraced(context.Background(), agent, fp, req, false)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if resp.Text != "recovered answer" {
		t.Errorf("final text = %q, want %q", resp.Text, "recovered answer")
	}
	if fp.calls != 3 {
		t.Errorf("provider calls = %d, want 3 (overflow + summarize + retry)", fp.calls)
	}
	if got := fp.requests[len(fp.requests)-1].ResumeSessionID; got != "" {
		t.Errorf("reactive fold retried stale CLI session %q", got)
	}
	if got := fp.requests[len(fp.requests)-1].CLIResumeScope; got != "scope" {
		t.Errorf("reactive fold lost durable scope %q", got)
	}
	if !hasRecovery(steps, string(contCompactRetry)) {
		t.Errorf("no reactive_compact_retry StepRecovery in trace: %+v", steps)
	}
}

// TestLoop_OverflowNoBoundary: when the history is too short to fold safely, the
// overflow is terminal (no infinite retry) and the error propagates.
func TestLoop_OverflowNoBoundary(t *testing.T) {
	rt := loopRuntime(t)
	agent := db.Agent{ID: "a1", Model: "m", MCPEnabled: true}
	fp := &fakeProvider{script: []scriptedResp{
		{err: errors.New("prompt is too long")},
	}}
	req := providers.Request{Messages: []providers.Message{{Role: providers.RoleUser, Text: "hi"}}}
	_, _, err := rt.CompleteWithToolsTraced(context.Background(), agent, fp, req, false)
	if err == nil {
		t.Fatalf("want terminal error, got nil")
	}
	if fp.calls != 1 {
		t.Errorf("provider calls = %d, want 1 (no retry without a safe fold boundary)", fp.calls)
	}
}
