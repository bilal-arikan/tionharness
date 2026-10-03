package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/flow"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// newFlowAgent creates a plain workspace agent for turn-level tests.
func newFlowAgent(t *testing.T, rt *Runtime, name string) db.Agent {
	t.Helper()
	a, err := rt.db.CreateAgent(context.Background(), db.Agent{Name: name})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	return a
}

// flowTestProvider answers each completion by looking at the LAST user message:
// the reply is picked by prompt substring, falling back to an echo. Every
// request is recorded so a test can assert what each node actually sent.
type flowTestProvider struct {
	mu       sync.Mutex
	requests []providers.Request
	reply    func(call int, req providers.Request) (*providers.Response, error)
}

func (p *flowTestProvider) Name() string { return "flow-test" }

func (p *flowTestProvider) Complete(_ context.Context, req providers.Request) (*providers.Response, error) {
	p.mu.Lock()
	p.requests = append(p.requests, req)
	call := len(p.requests)
	p.mu.Unlock()
	if p.reply != nil {
		return p.reply(call, req)
	}
	return &providers.Response{Text: "echo:" + lastUser(req), StopReason: providers.StopEndTurn, Model: "flow-model",
		Usage: providers.Usage{InputTokens: 10, OutputTokens: 5}}, nil
}

func lastUser(req providers.Request) string {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == providers.RoleUser {
			return req.Messages[i].Text
		}
	}
	return ""
}

var flowTestProviderOnce sync.Once
var flowTestCurrent *flowTestProvider

func installFlowTestProvider(t *testing.T, rt *Runtime, p *flowTestProvider) {
	t.Helper()
	flowTestProviderOnce.Do(func() {
		providers.RegisterKind(providers.NewBuiltinKind(
			providers.Manifest{Kind: "flow-test", Transport: providers.TransportAPI},
			func(providers.ResolvedConfig) bool { return true },
			func(providers.ResolvedConfig) (providers.Provider, error) { return flowTestCurrent, nil },
		))
	})
	flowTestCurrent = p
	rt.providers.SetInstances([]providers.Instance{{ID: "flow-test", KindID: "flow-test"}})
}

func flowTestAgent(t *testing.T, rt *Runtime, name string) db.Agent {
	t.Helper()
	a, err := rt.db.CreateAgent(context.Background(), db.Agent{Name: name, Provider: "flow-test", ProviderInstanceID: "flow-test", Model: "m"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	return a
}

func runTurn(t *testing.T, rt *Runtime, a db.Agent, input string) (*providers.Response, []TurnStep, error) {
	t.Helper()
	provider, err := rt.providers.Get(a.ProviderRef())
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	ctx := WithSessionID(context.Background(), "SES-flow")
	ctx = WithTurnID(ctx, "MSG-1")
	return rt.CompleteWithToolsStream(ctx, a, provider, providers.Request{
		Model:  a.Model,
		System: "sys",
		Messages: []providers.Message{
			{Role: providers.RoleUser, Text: "earlier question"},
			{Role: providers.RoleAssistant, Text: "earlier answer"},
			{Role: providers.RoleUser, Text: input},
		},
	}, false, nil)
}

// TestFlowTurnDefaultIsPlainTurn: the default flow is byte-for-byte the plain
// turn — one provider call with the untouched request — and records one run.
func TestFlowTurnDefaultIsPlainTurn(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	p := &flowTestProvider{}
	installFlowTestProvider(t, rt, p)
	a := flowTestAgent(t, rt, "A")
	ctx := context.Background()

	resp, steps, err := runTurn(t, rt, a, "merhaba")
	if err != nil {
		t.Fatalf("turn: %v", err)
	}
	if resp.Text != "echo:merhaba" || resp.Model != "flow-model" {
		t.Fatalf("resp = %+v", resp)
	}
	if len(p.requests) != 1 || len(p.requests[0].Messages) != 3 {
		t.Fatalf("plain turn must make one call with the full history, got %d calls", len(p.requests))
	}
	for _, s := range steps {
		if s.Kind == StepFlowNode {
			t.Fatalf("a trivial flow must not add node cards: %+v", s)
		}
	}
	f, err := rt.db.FlowForAgent(ctx, a.ID)
	if err != nil {
		t.Fatalf("flow not created: %v", err)
	}
	runs, _ := rt.db.ListFlowRuns(ctx, f.ID, 0)
	if len(runs) != 1 || runs[0].Status != db.FlowSuccess || runs[0].Input != "merhaba" || runs[0].Output != "echo:merhaba" {
		t.Fatalf("runs = %+v", runs)
	}
	if runs[0].SessionID != "SES-flow" || runs[0].MessageID != "MSG-1" || runs[0].Usage.LLMCalls != 1 || runs[0].Usage.InputTokens != 10 {
		t.Fatalf("run bookkeeping = %+v", runs[0])
	}
	if got, _ := rt.db.GetFlow(ctx, f.ID); got.Stats.Runs != 1 || got.Stats.Success != 1 {
		t.Fatalf("stats = %+v", got.Stats)
	}
}

func criticOps() []flow.Op {
	return []flow.Op{
		{Op: "insert_between", From: "respond", To: "output", Node: &flow.Node{ID: "critic", Type: flow.NodeLLM, Title: "Eleştiri", Context: flow.ContextFresh, Tools: flow.ToolsNone, Prompt: "Review: {{node.respond}}"}},
		{Op: "insert_between", From: "critic", To: "output", Node: &flow.Node{ID: "check", Type: flow.NodeRoute, Mode: flow.ModeContains, MaxVisits: 2}},
		{Op: "update_edge", ID: "e_check_output", Edge: &flow.Edge{When: "APPROVE"}},
		{Op: "add_edge", Edge: &flow.Edge{From: "check", To: "respond"}},
		{Op: "update_node", ID: "respond", Fields: json.RawMessage(`{"prompt":"{{input}}\n\nFeedback: {{node.critic}}"}`)},
		{Op: "update_node", ID: "output", Fields: json.RawMessage(`{"template":"{{node.respond}}"}`)},
	}
}

// TestFlowTurnCriticLoop drives a draft → critic → route → draft loop end to
// end: thread nodes carry the history, fresh nodes do not, the route loops
// once and the reply is the second draft.
func TestFlowTurnCriticLoop(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	p := &flowTestProvider{}
	p.reply = func(call int, req providers.Request) (*providers.Response, error) {
		u := lastUser(req)
		text := "draft"
		switch {
		case strings.HasPrefix(u, "Review:") && strings.Contains(u, "draft-v2"):
			text = "APPROVE"
		case strings.HasPrefix(u, "Review:"):
			text = "REVISE: too short"
		case strings.Contains(u, "Feedback: REVISE"):
			text = "draft-v2"
		}
		return &providers.Response{Text: text, StopReason: providers.StopEndTurn, Model: "flow-model", Usage: providers.Usage{InputTokens: 1, OutputTokens: 1}, SessionID: "cli-" + text}, nil
	}
	installFlowTestProvider(t, rt, p)
	a := flowTestAgent(t, rt, "B")
	ctx := context.Background()
	f, err := rt.db.EnsureAgentFlow(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.ApplyFlowOps(ctx, f.ID, criticOps(), db.FlowAuthor{Kind: db.FlowAuthorAgent, ID: a.ID}, "add critic", ""); err != nil {
		t.Fatalf("apply ops: %v", err)
	}

	var live []TurnStep
	provider, _ := rt.providers.Get(a.ProviderRef())
	ctx2 := WithSessionID(context.Background(), "SES-2")
	resp, steps, err := rt.CompleteWithToolsStream(ctx2, a, provider, providers.Request{
		Model: a.Model, System: "sys",
		Messages: []providers.Message{{Role: providers.RoleUser, Text: "q1"}, {Role: providers.RoleAssistant, Text: "a1"}, {Role: providers.RoleUser, Text: "write"}},
	}, false, func(st TurnStep) { live = append(live, st) })
	if err != nil {
		t.Fatalf("turn: %v", err)
	}
	if resp.Text != "draft-v2" {
		t.Fatalf("reply = %q", resp.Text)
	}
	// respond, critic, respond, critic = 4 model calls
	if len(p.requests) != 4 {
		t.Fatalf("calls = %d", len(p.requests))
	}
	// thread node: full history, prompt replaces the last user message.
	if n := len(p.requests[0].Messages); n != 3 || lastUser(p.requests[0]) != "write\n\nFeedback: " {
		t.Fatalf("thread node request wrong: %d msgs, last %q", n, lastUser(p.requests[0]))
	}
	// fresh node: only its prompt, no history, no resume id.
	if n := len(p.requests[1].Messages); n != 1 || p.requests[1].ResumeSessionID != "" {
		t.Fatalf("fresh node request wrong: %d msgs resume=%q", n, p.requests[1].ResumeSessionID)
	}
	// second draft saw the critic's note
	if !strings.Contains(lastUser(p.requests[2]), "Feedback: REVISE: too short") {
		t.Fatalf("second draft prompt = %q", lastUser(p.requests[2]))
	}
	// usage summed, resume id from the last THREAD node, not the critic
	if resp.Usage.InputTokens != 4 || resp.SessionID != "cli-draft-v2" || resp.ProviderCalls != 4 {
		t.Fatalf("resp bookkeeping = usage %+v session %q calls %d", resp.Usage, resp.SessionID, resp.ProviderCalls)
	}
	// node cards: input, respond, critic, check, respond, critic, check, output = 8 persisted
	cards := 0
	for _, s := range steps {
		if s.Kind == StepFlowNode {
			cards++
			if s.Running {
				t.Fatalf("persisted node card must not be running: %+v", s)
			}
		}
	}
	if cards != 8 {
		t.Fatalf("node cards = %d, steps=%+v", cards, steps)
	}
	if len(live) < 16 {
		t.Fatalf("live frames = %d (start+done per node)", len(live))
	}
	runs, _ := rt.db.ListFlowRuns(ctx, f.ID, 0)
	if len(runs) != 1 || runs[0].StepCount != 8 || runs[0].Version != 2 || runs[0].Status != db.FlowSuccess {
		t.Fatalf("run = %+v", runs[0])
	}
	var trace []flow.Step
	if err := json.Unmarshal(runs[0].Steps, &trace); err != nil {
		t.Fatal(err)
	}
	if trace[3].Edge != "*" || trace[6].Edge != "APPROVE" {
		t.Fatalf("route arms = %q %q", trace[3].Edge, trace[6].Edge)
	}
}

// TestFlowTurnKeepsPeerReplyTail: a multi-agent session hands the second
// agent a request whose last message is the first agent's reply. The default
// flow must send it untouched — no duplicated user message (e2e multi-agent).
func TestFlowTurnKeepsPeerReplyTail(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	p := &flowTestProvider{}
	p.reply = func(call int, req providers.Request) (*providers.Response, error) {
		return &providers.Response{Text: "Bryn: agreed", StopReason: providers.StopEndTurn}, nil
	}
	installFlowTestProvider(t, rt, p)
	a := flowTestAgent(t, rt, "Bryn")
	provider, _ := rt.providers.Get(a.ProviderRef())
	msgs := []providers.Message{
		{Role: providers.RoleUser, Text: "How should we speed up the catalog?"},
		{Role: providers.RoleAssistant, Text: "Ada: I propose we cache the catalog."},
	}
	ctx := WithSessionID(context.Background(), "SES-peer")
	if _, _, err := rt.CompleteWithToolsStream(ctx, a, provider, providers.Request{Model: a.Model, System: "sys", Messages: msgs}, false, nil); err != nil {
		t.Fatalf("turn: %v", err)
	}
	if len(p.requests) != 1 || len(p.requests[0].Messages) != 2 {
		t.Fatalf("the peer reply tail must go out untouched: %+v", p.requests)
	}
	// Grading / hooks / observer run detached; let them finish before the DB closes.
	rt.spawnWG.Wait()
}

// TestFlowTurnNodeFailureFailsRun: a provider error inside a node fails the
// turn and the run row records the failure with the partial trace.
func TestFlowTurnNodeFailureFailsRun(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	p := &flowTestProvider{}
	p.reply = func(call int, req providers.Request) (*providers.Response, error) {
		if strings.HasPrefix(lastUser(req), "Review:") {
			return nil, errors.New("critic down")
		}
		return &providers.Response{Text: "draft", StopReason: providers.StopEndTurn}, nil
	}
	installFlowTestProvider(t, rt, p)
	a := flowTestAgent(t, rt, "C")
	ctx := context.Background()
	f, _ := rt.db.EnsureAgentFlow(ctx, a.ID)
	if _, err := rt.ApplyFlowOps(ctx, f.ID, criticOps(), db.FlowAuthor{Kind: db.FlowAuthorUser}, "x", ""); err != nil {
		t.Fatal(err)
	}
	_, _, err := runTurn(t, rt, a, "go")
	if err == nil || !strings.Contains(err.Error(), "critic down") {
		t.Fatalf("err = %v", err)
	}
	runs, _ := rt.db.ListFlowRuns(ctx, f.ID, 0)
	if len(runs) != 1 || runs[0].Status != db.FlowFailure || !strings.Contains(runs[0].Error, "critic") || runs[0].StepCount != 3 {
		t.Fatalf("run = %+v", runs[0])
	}
	if got, _ := rt.db.GetFlow(ctx, f.ID); got.Stats.Failure != 1 {
		t.Fatalf("stats = %+v", got.Stats)
	}
}

// TestFlowVersioningAndBudget: ops commit versions with a diff, the growth
// budget stops agents/observer but not the user, revert makes a new head and
// prompt edits are versioned and restorable.
func TestFlowVersioningAndBudget(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	a := newFlowAgent(t, rt, "D")
	ctx := context.Background()
	f, _ := rt.db.EnsureAgentFlow(ctx, a.ID)
	v, err := rt.ApplyFlowOps(ctx, f.ID, criticOps(), db.FlowAuthor{Kind: db.FlowAuthorAgent, ID: a.ID}, "critic", "")
	if err != nil {
		t.Fatal(err)
	}
	if v.Version != 2 || v.Parent != 1 || !strings.Contains(v.Diff, "+critic") {
		t.Fatalf("version = %+v", v)
	}
	// Budget: policy cap 16 by default; shrink it and try to add a node as agent.
	pol := db.DefaultFlowPolicy()
	pol.MaxNodes = 5
	if _, err := rt.db.UpdateFlowMeta(ctx, f.ID, nil, nil, &pol); err != nil {
		t.Fatal(err)
	}
	extra := []flow.Op{{Op: "insert_between", From: "input", To: "respond", Node: &flow.Node{ID: "plan", Type: flow.NodeLLM, Prompt: "plan {{input}}"}}}
	if _, err := rt.ApplyFlowOps(ctx, f.ID, extra, db.FlowAuthor{Kind: db.FlowAuthorAgent, ID: a.ID}, "plan", ""); err == nil || !strings.Contains(err.Error(), "growth budget") {
		t.Fatalf("agent must hit the growth budget, got %v", err)
	}
	if _, err := rt.ApplyFlowOps(ctx, f.ID, extra, db.FlowAuthor{Kind: db.FlowAuthorUser}, "plan", ""); err != nil {
		t.Fatalf("user is not budget-bound: %v", err)
	}
	// No-op save is refused.
	cur, _ := rt.db.GetFlow(ctx, f.ID)
	g, _ := flow.Parse(cur.Graph)
	if _, err := rt.SaveFlowGraph(ctx, f.ID, g, db.FlowAuthor{Kind: db.FlowAuthorUser}, "same"); err == nil {
		t.Fatal("saving the same graph must report no change")
	}
	// Revert to v1 = head v4 with the default graph.
	rv, err := rt.RevertFlow(ctx, f.ID, 1, db.FlowAuthor{Kind: db.FlowAuthorUser}, "")
	if err != nil {
		t.Fatal(err)
	}
	cur, _ = rt.db.GetFlow(ctx, f.ID)
	g, _ = flow.Parse(cur.Graph)
	if rv.Version != 4 || cur.Version != 4 || !g.IsTrivial() {
		t.Fatalf("revert: v%d trivial=%v", cur.Version, g.IsTrivial())
	}
	vs, _ := rt.db.ListFlowVersions(ctx, f.ID)
	if len(vs) != 4 || vs[0].Version != 4 {
		t.Fatalf("versions = %d", len(vs))
	}
	// Prompt evolution: v1 snapshot + v2 change; restore v1 → v3.
	soul := "Yeni ruh"
	if err := rt.UpdateAgentPrompts(ctx, a.ID, &soul, nil, db.FlowAuthor{Kind: db.FlowAuthorAgent, ID: a.ID}, "lesson", ""); err != nil {
		t.Fatal(err)
	}
	pv, _ := rt.db.ListAgentPromptVersions(ctx, a.ID)
	if len(pv) != 2 || pv[0].Soul != "Yeni ruh" || pv[1].Version != 1 {
		t.Fatalf("prompt versions = %+v", pv)
	}
	if got, _ := rt.db.GetAgent(ctx, a.ID); got.Soul != "Yeni ruh" {
		t.Fatalf("soul not applied: %q", got.Soul)
	}
	if err := rt.RestoreAgentPromptVersion(ctx, a.ID, 1, db.FlowAuthor{Kind: db.FlowAuthorUser}); err != nil {
		t.Fatal(err)
	}
	if got, _ := rt.db.GetAgent(ctx, a.ID); got.Soul != pv[1].Soul {
		t.Fatalf("restore did not put the old soul back: %q", got.Soul)
	}
}

// TestFlowObserverFilesAndAppliesProposal: the observer pass parses the model's
// JSON, validates ops in code, files a proposal and (policy auto) applies it.
func TestFlowObserverFilesAndAppliesProposal(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	p := &flowTestProvider{}
	proposal := `{"noChange":false,"reason":"3/3 runs had no review","expected":"fewer wrong answers","confidence":0.9,"ops":[{"op":"insert_between","from":"respond","to":"output","node":{"id":"critic","type":"llm","context":"fresh","tools":"none","prompt":"Review: {{node.respond}}"}}],"prompt":{"identity":"Kısa ve net yanıt ver."}}`
	p.reply = func(call int, req providers.Request) (*providers.Response, error) {
		if strings.Contains(req.System, "Flow Observer") || strings.Contains(lastUser(req), "# Flow") {
			return &providers.Response{Text: proposal, StopReason: providers.StopEndTurn}, nil
		}
		return &providers.Response{Text: "ok", StopReason: providers.StopEndTurn}, nil
	}
	installFlowTestProvider(t, rt, p)
	a := flowTestAgent(t, rt, "E")
	ctx := context.Background()
	f, _ := rt.db.EnsureAgentFlow(ctx, a.ID)
	// Seed three runs so the observer has evidence.
	for range 3 {
		if _, _, err := runTurn(t, rt, a, "hello"); err != nil {
			t.Fatal(err)
		}
	}
	// Policy propose: a proposal is filed, nothing applied.
	res, err := rt.OptimizeFlow(ctx, f.ID, "manual")
	if err != nil {
		t.Fatalf("optimize: %v", err)
	}
	if !res.Ran || res.Proposal == nil || res.Proposal.Status != db.ProposalPending || res.Applied {
		t.Fatalf("result = %+v", res)
	}
	if cur, _ := rt.db.GetFlow(ctx, f.ID); cur.Version != 1 || cur.Stats.RunsAtOptimize != 3 {
		t.Fatalf("propose mode must not apply; flow = v%d marker=%d", cur.Version, cur.Stats.RunsAtOptimize)
	}
	applied, err := rt.ApplyFlowProposal(ctx, res.Proposal.ID, db.FlowAuthor{Kind: db.FlowAuthorUser})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if applied.Status != db.ProposalApplied || applied.AppliedVersion != 2 {
		t.Fatalf("applied = %+v", applied)
	}
	if got, _ := rt.db.GetAgent(ctx, a.ID); got.Identity != "Kısa ve net yanıt ver." {
		t.Fatalf("identity not applied: %q", got.Identity)
	}
	// Policy auto: the next pass applies by itself (confidence 0.9 ≥ 0.7).
	pol := db.DefaultFlowPolicy()
	pol.Mode = db.FlowPolicyAuto
	if _, err := rt.db.UpdateFlowMeta(ctx, f.ID, nil, nil, &pol); err != nil {
		t.Fatal(err)
	}
	// Reverting first so the same ops apply cleanly again.
	if _, err := rt.RevertFlow(ctx, f.ID, 1, db.FlowAuthor{Kind: db.FlowAuthorUser}, ""); err != nil {
		t.Fatal(err)
	}
	res, err = rt.OptimizeFlow(ctx, f.ID, "auto")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Applied || res.Proposal == nil || res.Proposal.Status != db.ProposalApplied {
		t.Fatalf("auto result = %+v", res)
	}
	if cur, _ := rt.db.GetFlow(ctx, f.ID); cur.Version != 4 {
		t.Fatalf("auto apply must commit a new head, got v%d", cur.Version)
	}
	// A proposal that breaks the graph is filed as invalid, never applied.
	proposal = `{"noChange":false,"reason":"bad","confidence":0.95,"ops":[{"op":"remove_node","id":"output"}]}`
	res, err = rt.OptimizeFlow(ctx, f.ID, "manual")
	if err != nil {
		t.Fatal(err)
	}
	if res.Proposal == nil || res.Proposal.Status != db.ProposalInvalid || res.Applied {
		t.Fatalf("invalid proposal handling = %+v", res)
	}
	// Feedback on the message lands on the run.
	runs, _ := rt.db.ListFlowRuns(ctx, f.ID, 0)
	sess, _ := rt.db.CreateSession(ctx, db.Session{AgentID: a.ID})
	msg, _ := rt.db.AddMessage(ctx, db.Message{SessionID: sess.ID, Role: "assistant", Text: "x", ID: "MSG-fb"})
	if _, err := rt.db.CreateFlowRun(ctx, db.FlowRun{FlowID: f.ID, MessageID: msg.ID, SessionID: sess.ID}); err != nil {
		t.Fatal(err)
	}
	if err := rt.db.SetMessageFeedback(ctx, sess.ID, msg.ID, -1, ""); err != nil {
		t.Fatal(err)
	}
	runs, _ = rt.db.ListFlowRuns(ctx, f.ID, 0)
	if runs[0].Feedback != -1 {
		t.Fatalf("feedback not mirrored: %+v", runs[0])
	}
	drainSpawns(t, rt)
}
