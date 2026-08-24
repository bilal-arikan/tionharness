package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// scriptedResp is one programmed reply from fakeProvider.
type scriptedResp struct {
	text string
	stop string
	err  error
}

// fakeProvider returns a fixed sequence of replies, letting a test drive the
// native tool loop through its recovery branches deterministically (no key, no
// network). Calls past the script return a plain end_turn.
type fakeProvider struct {
	calls  int
	script []scriptedResp
}

func (f *fakeProvider) Name() string { return "fake" }

func (f *fakeProvider) Complete(_ context.Context, _ providers.Request) (*providers.Response, error) {
	i := f.calls
	f.calls++
	if i >= len(f.script) {
		return &providers.Response{StopReason: providers.StopEndTurn, Text: ""}, nil
	}
	s := f.script[i]
	if s.err != nil {
		return nil, s.err
	}
	return &providers.Response{StopReason: s.stop, Text: s.text}, nil
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

	req := providers.Request{Messages: msgs}
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
