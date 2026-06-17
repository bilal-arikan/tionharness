package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/providers"
	"github.com/bilal/swarmgo/internal/tools"
)

// TestCallAgentGate verifies call_agent is absent until delegation is enabled.
func TestCallAgentGate(t *testing.T) {
	rt, tun := newTestRuntime(t, t.TempDir())
	ctx := context.Background()
	agent, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Caller", Provider: "anthropic", MCPEnabled: true})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	if rt.buildRegistry(ctx, agent).Has("call_agent") {
		t.Fatal("call_agent must be absent when delegation is disabled")
	}
	tun.SetDelegationEnabled(true)
	if !rt.buildRegistry(ctx, agent).Has("call_agent") {
		t.Fatal("call_agent must be present when delegation is enabled")
	}
}

// runnerFor seeds a chain position into ctx and returns the delegation runner.
func runnerFor(t *testing.T, rt *Runtime, caller db.Agent, st delegState) tools.DelegateRunner {
	t.Helper()
	ctx := context.WithValue(context.Background(), delegStateKey{}, st)
	var req providers.Request
	ctx = rt.withDelegation(ctx, caller, &req, false)
	run := tools.DelegationFrom(ctx)
	if run == nil {
		t.Fatal("expected a delegation runner in context")
	}
	return run
}

// TestDelegationGuards exercises the three loop-protection mechanisms: each must
// refuse BEFORE the sub-agent runs (so no provider call is needed).
func TestDelegationGuards(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	ctx := context.Background()
	caller, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "Caller", Provider: "anthropic"})
	target, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "Helper", Provider: "anthropic"})

	t.Run("cycle/self", func(t *testing.T) {
		n := 0
		run := runnerFor(t, rt, caller, delegState{depth: 0, visited: map[string]bool{caller.ID: true}, calls: &n})
		if _, err := run(context.Background(), caller.Name, "do it"); err == nil ||
			!strings.Contains(err.Error(), "already part of this delegation chain") {
			t.Fatalf("expected cycle guard to refuse self, got %v", err)
		}
	})

	t.Run("depth", func(t *testing.T) {
		n := 0
		st := delegState{depth: DefaultMaxDelegationDepth, visited: map[string]bool{caller.ID: true}, calls: &n}
		run := runnerFor(t, rt, caller, st)
		if _, err := run(context.Background(), target.Name, "do it"); err == nil ||
			!strings.Contains(err.Error(), "depth limit") {
			t.Fatalf("expected depth guard to refuse, got %v", err)
		}
	})

	t.Run("budget", func(t *testing.T) {
		n := DefaultMaxDelegationCalls
		st := delegState{depth: 0, visited: map[string]bool{caller.ID: true}, calls: &n}
		run := runnerFor(t, rt, caller, st)
		if _, err := run(context.Background(), target.Name, "do it"); err == nil ||
			!strings.Contains(err.Error(), "budget") {
			t.Fatalf("expected budget guard to refuse, got %v", err)
		}
	})

	t.Run("unknown agent", func(t *testing.T) {
		n := 0
		st := delegState{depth: 0, visited: map[string]bool{caller.ID: true}, calls: &n}
		run := runnerFor(t, rt, caller, st)
		if _, err := run(context.Background(), "Ghost", "do it"); err == nil ||
			!strings.Contains(err.Error(), "no agent named") {
			t.Fatalf("expected unknown-agent error, got %v", err)
		}
	})
}

// TestInheritedMessages keeps readable turns and drops tool plumbing so the
// summoned agent never receives a dangling tool_use.
func TestInheritedMessages(t *testing.T) {
	req := &providers.Request{Messages: []providers.Message{
		{Role: providers.RoleUser, Text: "hi"},
		{Role: providers.RoleAssistant, Text: "thinking", ToolCalls: []providers.ToolCall{{ID: "x", Name: "call_agent"}}},
		{Role: providers.RoleUser, ToolResults: []providers.ToolResult{{CallID: "x", Content: "result"}}},
		{Role: providers.RoleAssistant, Text: "final"},
	}}
	got := inheritedMessages(req)
	if len(got) != 3 {
		t.Fatalf("expected 3 readable turns, got %d: %+v", len(got), got)
	}
	for _, m := range got {
		if len(m.ToolCalls) != 0 || len(m.ToolResults) != 0 {
			t.Fatalf("tool plumbing leaked into inherited messages: %+v", m)
		}
	}
}
