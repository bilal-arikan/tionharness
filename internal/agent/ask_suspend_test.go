package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// scriptedToolResp is one programmed reply that can carry tool calls, so a test
// can drive the native loop into an ask_user tool_use.
type scriptedToolResp struct {
	stop      string
	text      string
	toolCalls []providers.ToolCall
}

type toolFakeProvider struct {
	calls  int
	script []scriptedToolResp
}

func (f *toolFakeProvider) Name() string { return "fake" }

func (f *toolFakeProvider) Complete(_ context.Context, _ providers.Request) (*providers.Response, error) {
	i := f.calls
	f.calls++
	if i >= len(f.script) {
		return &providers.Response{StopReason: providers.StopEndTurn}, nil
	}
	s := f.script[i]
	return &providers.Response{StopReason: s.stop, Text: s.text, ToolCalls: s.toolCalls}, nil
}

func askCall() []providers.ToolCall {
	return []providers.ToolCall{{ID: "ask-1", Name: askUserToolName, Input: json.RawMessage(`{"question":"Proceed?"}`)}}
}

// TestDurableAsk_SuspendAtCleanPoint verifies the gated suspend seam: with
// WithDurableAsk on ctx, a lone native ask_user call parks the turn (returns an
// *askSuspend sentinel carrying the call id + card payload) instead of blocking.
func TestDurableAsk_SuspendAtCleanPoint(t *testing.T) {
	rt := loopRuntime(t)
	agent := db.Agent{ID: "a1", Model: "m", MCPEnabled: true}
	fp := &toolFakeProvider{script: []scriptedToolResp{
		{stop: providers.StopToolUse, toolCalls: askCall()},
	}}

	req := providers.Request{Messages: []providers.Message{{Role: providers.RoleUser, Text: "should I?"}}}
	_, _, err := rt.CompleteWithToolsStream(WithDurableAsk(context.Background()), agent, fp, req, false, nil)

	var sus *askSuspend
	if !errors.As(err, &sus) {
		t.Fatalf("expected *askSuspend, got %v", err)
	}
	if sus.CallID != "ask-1" {
		t.Errorf("suspend CallID = %q, want ask-1", sus.CallID)
	}
	if !strings.Contains(string(sus.Payload), "Proceed?") {
		t.Errorf("suspend payload missing question: %s", sus.Payload)
	}
	// The suspended history must include the assistant tool_use turn (so resume can
	// answer it): user prompt + assistant tool_use = 2 messages.
	if len(sus.Messages) != 2 || len(sus.Messages[1].ToolCalls) != 1 {
		t.Fatalf("suspended history should end with the assistant tool_use turn, got %d msgs", len(sus.Messages))
	}
}

// TestDurableAsk_DisabledDoesNotSuspend verifies the gate: WITHOUT WithDurableAsk,
// the same ask_user call does NOT park — it falls through to the ordinary path
// (no asker present → the tool errors back into the loop, which continues and
// finishes normally), exactly as before this feature.
func TestDurableAsk_DisabledDoesNotSuspend(t *testing.T) {
	rt := loopRuntime(t)
	ctx := context.Background()
	agent, err := rt.db.CreateAgent(ctx, db.Agent{Name: "A", Provider: "anthropic", Model: "m", MCPEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := rt.db.CreateSession(ctx, db.Session{AgentID: agent.ID, Title: "T"})
	if err != nil {
		t.Fatal(err)
	}
	fp := &toolFakeProvider{script: []scriptedToolResp{
		{stop: providers.StopToolUse, toolCalls: askCall()},
		{stop: providers.StopEndTurn, text: "done anyway"},
	}}

	req := providers.Request{Messages: []providers.Message{{Role: providers.RoleUser, Text: "should I?"}}}
	resp, _, err := rt.CompleteWithToolsStream(WithSessionID(ctx, sess.ID), agent, fp, req, false, nil)
	if err != nil {
		t.Fatalf("no-suspend path should complete, got %v", err)
	}
	var sus *askSuspend
	if errors.As(err, &sus) {
		t.Fatal("must not suspend without WithDurableAsk")
	}
	if resp.Text != "done anyway" {
		t.Errorf("final text = %q, want %q", resp.Text, "done anyway")
	}
	events, err := rt.db.ReadDebugEvents(ctx, sess.ID, db.DebugTool, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || !events[0].Err || events[0].Error == "" {
		t.Fatalf("failed tool debug event missing error text: %+v", events)
	}
	if events[0].Args != "[redacted tool input]" || strings.Contains(events[0].Args, "Proceed?") {
		t.Fatalf("failed tool debug event leaked raw arguments: %+v", events[0])
	}
}

// TestDurableAsk_PersistAndResumeRoundTrip drives the full MVP round trip: suspend
// → persist → claim(answer) → re-drive folds the answer as the tool_result and the
// turn completes, with the pre-suspend trace prepended.
func TestDurableAsk_PersistAndResumeRoundTrip(t *testing.T) {
	rt := loopRuntime(t)
	ctx := context.Background()
	agent, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Asker", Provider: "anthropic", Model: "m", MCPEnabled: true})
	if err != nil {
		t.Fatal(err)
	}

	// 1) Suspend.
	fp := &toolFakeProvider{script: []scriptedToolResp{{stop: providers.StopToolUse, toolCalls: askCall()}}}
	req := providers.Request{Model: "m", System: "SYS", Messages: []providers.Message{{Role: providers.RoleUser, Text: "should I?"}}}
	preSteps := []TurnStep{{Kind: StepText, Text: "thinking..."}}
	_, _, serr := rt.CompleteWithToolsStream(WithDurableAsk(ctx), agent, fp, req, false, nil)
	var sus *askSuspend
	if !errors.As(serr, &sus) {
		t.Fatalf("expected suspend, got %v", serr)
	}

	// 2) Persist the suspend snapshot and verify it round-trips through the store.
	ask, err := rt.persistAskSuspend(ctx, agent, "SES1", req, preSteps, sus)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := rt.db.GetSessionAsk(ctx, ask.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.CallID != "ask-1" || reopened.Status != db.SessionAskWaiting {
		t.Fatalf("persisted ask mismatch: %+v", reopened)
	}

	// 3) Claim (single-winner) then re-drive with a provider that finishes the turn.
	claimed, err := rt.db.ClaimSessionAsk(ctx, ask.ID, "yes, proceed")
	if err != nil {
		t.Fatal(err)
	}
	fp2 := &toolFakeProvider{script: []scriptedToolResp{{stop: providers.StopEndTurn, text: "all done"}}}
	resp, steps, reSuspend, err := rt.driveResumedAsk(ctx, claimed, agent, fp2, "yes, proceed", nil)
	if err != nil {
		t.Fatalf("resume drive: %v", err)
	}
	if reSuspend != nil {
		t.Fatalf("clean finish should not re-suspend, got %+v", reSuspend)
	}
	if resp.Text != "all done" {
		t.Errorf("resumed text = %q, want %q", resp.Text, "all done")
	}
	// Pre-suspend trace is prepended.
	if len(steps) == 0 || steps[0].Kind != StepText || steps[0].Text != "thinking..." {
		t.Errorf("pre-suspend step should lead the resumed trace, got %+v", steps)
	}
	// The folded answer must have re-entered the loop as the ask's tool_result: the
	// fake ignores content, but a second claim must now fail (already resolved).
	if _, err := rt.db.ClaimSessionAsk(ctx, ask.ID, "again"); err == nil {
		t.Error("second claim should fail after resume")
	}
}

// TestDurableAsk_ResumeReSuspends verifies the follow-up-ask case: a resumed turn
// that asks again at a clean point re-parks a NEW durable ask (carrying the full
// accumulated trace) instead of finishing.
func TestDurableAsk_ResumeReSuspends(t *testing.T) {
	rt := loopRuntime(t)
	ctx := context.Background()
	agent, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Asker", Provider: "anthropic", Model: "m", MCPEnabled: true})
	if err != nil {
		t.Fatal(err)
	}

	// First suspend.
	fp := &toolFakeProvider{script: []scriptedToolResp{{stop: providers.StopToolUse, toolCalls: askCall()}}}
	req := providers.Request{Model: "m", Messages: []providers.Message{{Role: providers.RoleUser, Text: "q1"}}}
	_, _, serr := rt.CompleteWithToolsStream(WithDurableAsk(ctx), agent, fp, req, false, nil)
	var sus *askSuspend
	if !errors.As(serr, &sus) {
		t.Fatalf("expected first suspend, got %v", serr)
	}
	ask, err := rt.persistAskSuspend(ctx, agent, "SES1", req, nil, sus)
	if err != nil {
		t.Fatal(err)
	}
	claimed, _ := rt.db.ClaimSessionAsk(ctx, ask.ID, "answer 1")

	// Resume with a provider that asks AGAIN → the turn re-parks a new ask.
	fp2 := &toolFakeProvider{script: []scriptedToolResp{{stop: providers.StopToolUse, toolCalls: askCall()}}}
	resp, _, reSuspend, err := rt.driveResumedAsk(ctx, claimed, agent, fp2, "answer 1", nil)
	if err != nil {
		t.Fatalf("resume drive: %v", err)
	}
	if resp != nil {
		t.Errorf("re-suspend should not return a response, got %+v", resp)
	}
	if reSuspend == nil {
		t.Fatal("a follow-up clean ask should re-park a new durable ask")
	}
	if reSuspend.Status != db.SessionAskWaiting || reSuspend.ID == ask.ID {
		t.Fatalf("re-suspend should be a fresh waiting ask, got %+v", reSuspend)
	}
}

// TestDurablePermission_SuspendAtCleanPoint verifies the permission analog: under
// "ask" mode a lone write/exec call that would prompt is parked (Kind="permission",
// carrying the full call for execute-on-resume) instead of blocking the prompter.
func TestDurablePermission_SuspendAtCleanPoint(t *testing.T) {
	rt := loopRuntime(t)
	agent := db.Agent{ID: "a1", Model: "m", MCPEnabled: true, PermissionMode: "ask"}
	fp := &toolFakeProvider{script: []scriptedToolResp{
		{stop: providers.StopToolUse, toolCalls: []providers.ToolCall{
			{ID: "w-1", Name: "Write", Input: json.RawMessage(`{"path":"x.txt","content":"hi"}`)},
		}},
	}}
	ctx := WithDurableAsk(context.Background())
	ctx = tools.WithPermissionPrompter(ctx, func(context.Context, string, string, string, []string) (string, error) {
		return "allow", nil // present so wouldPromptPermission is true; never reached (we suspend)
	})
	req := providers.Request{Messages: []providers.Message{{Role: providers.RoleUser, Text: "write it"}}}
	_, _, err := rt.CompleteWithToolsStream(ctx, agent, fp, req, false, nil)

	var sus *askSuspend
	if !errors.As(err, &sus) {
		t.Fatalf("expected *askSuspend, got %v", err)
	}
	if sus.Kind != "permission" || sus.Call.Name != "Write" {
		t.Fatalf("expected a permission suspend for Write, got kind=%q call=%q", sus.Kind, sus.Call.Name)
	}
	if !strings.Contains(string(sus.Payload), "Write") {
		t.Errorf("permission card payload missing tool: %s", sus.Payload)
	}
}

// TestResolveResumeResult_PermissionDeny verifies a denied permission resume
// synthesizes the same denial tool_result the live gate feeds the model — without
// executing the call.
func TestResolveResumeResult_PermissionDeny(t *testing.T) {
	rt := loopRuntime(t)
	state := askSuspendState{Kind: "permission", CallID: "w-1", Call: providers.ToolCall{ID: "w-1", Name: "Write"}}
	res, step := rt.resolveResumeResult(context.Background(), db.Agent{ID: "a1"}, state, "deny")
	if !res.IsError || !strings.Contains(res.Content, "denied") {
		t.Fatalf("deny should produce an error tool_result, got %+v", res)
	}
	if step == nil || !step.IsError {
		t.Fatalf("deny should record an error step, got %+v", step)
	}
}

// TestSweepWaitingAsks verifies the timeout sweeper: a card past its TimeoutSec is
// CAS-closed to timeout; a card before its deadline and a TimeoutSec=0 (unlimited)
// card both survive.
func TestSweepWaitingAsks(t *testing.T) {
	rt := loopRuntime(t)
	ctx := context.Background()

	bounded, _ := rt.db.CreateSessionAsk(ctx, db.SessionAsk{SessionID: "S", State: "{}", TimeoutSec: 10})
	rt.sweepWaitingAsksAt(ctx, bounded.CreatedAt+5) // before deadline
	if got, _ := rt.db.GetSessionAsk(ctx, bounded.ID); got.Status != db.SessionAskWaiting {
		t.Fatalf("ask before deadline should survive, got %q", got.Status)
	}
	rt.sweepWaitingAsksAt(ctx, bounded.CreatedAt+15) // past deadline
	if got, _ := rt.db.GetSessionAsk(ctx, bounded.ID); got.Status != db.SessionAskTimeout {
		t.Fatalf("ask past deadline should time out, got %q", got.Status)
	}

	unlimited, _ := rt.db.CreateSessionAsk(ctx, db.SessionAsk{SessionID: "S", State: "{}", TimeoutSec: 0})
	rt.sweepWaitingAsksAt(ctx, unlimited.CreatedAt+int64(1)<<30) // far future
	if got, _ := rt.db.GetSessionAsk(ctx, unlimited.ID); got.Status != db.SessionAskWaiting {
		t.Fatalf("TimeoutSec=0 must never time out, got %q", got.Status)
	}
}
