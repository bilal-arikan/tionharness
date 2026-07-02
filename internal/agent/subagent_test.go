package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bilal-arikan/swarmgo/internal/db"
	"github.com/bilal-arikan/swarmgo/internal/tools"
)

// runAgentFor seeds a chain position into ctx and returns a runner already bound
// to that ctx — runAgent reads the chain position from the call-time context (as
// the real tool loop passes it), so the seeded ctx must reach the runner.
func runAgentFor(t *testing.T, rt *Runtime, caller db.Agent, st delegState) func(tools.RunAgentSpec) (tools.RunAgentResult, error) {
	t.Helper()
	ctx := context.WithValue(context.Background(), delegStateKey{}, st)
	ctx = rt.withRunAgent(ctx, caller, nil, false)
	run := tools.RunAgentFrom(ctx)
	if run == nil {
		t.Fatal("expected a run-agent runner in context")
	}
	return func(spec tools.RunAgentSpec) (tools.RunAgentResult, error) { return run(ctx, spec) }
}

// TestRunSubagentAlwaysInstalled verifies run_subagent is shipped by default
// (2026-07-02: the app-settings delegation master toggle was removed; the tool is
// always installed and availability is managed per-tool from the Tools screen).
func TestRunSubagentAlwaysInstalled(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	ctx := context.Background()
	agent, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Caller", Provider: "anthropic", MCPEnabled: true})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if !rt.buildRegistry(ctx, agent).Has("run_subagent") {
		t.Fatal("run_subagent must always be present (per-tool visibility handles disabling)")
	}
}

// TestRunAgentGuards exercises depth, budget and cycle guards — each must refuse
// before any provider call.
func TestRunAgentGuards(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	ctx := context.Background()
	caller, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "Caller", Provider: "anthropic"})
	target, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "Helper", Provider: "anthropic"})

	t.Run("depth", func(t *testing.T) {
		var n int32
		run := runAgentFor(t, rt, caller, delegState{depth: DefaultMaxDelegationDepth, visited: map[string]bool{caller.ID: true}, calls: &n})
		if _, err := run(tools.RunAgentSpec{Target: target.Name, Task: "x"}); err == nil || !strings.Contains(err.Error(), "depth limit") {
			t.Fatalf("expected depth guard, got %v", err)
		}
	})

	t.Run("budget", func(t *testing.T) {
		var n int32 = DefaultMaxDelegationCalls
		run := runAgentFor(t, rt, caller, delegState{depth: 0, visited: map[string]bool{caller.ID: true}, calls: &n})
		if _, err := run(tools.RunAgentSpec{Target: target.Name, Task: "x"}); err == nil || !strings.Contains(err.Error(), "budget") {
			t.Fatalf("expected budget guard, got %v", err)
		}
	})

	t.Run("cycle/self", func(t *testing.T) {
		var n int32
		run := runAgentFor(t, rt, caller, delegState{depth: 0, visited: map[string]bool{caller.ID: true}, calls: &n})
		if _, err := run(tools.RunAgentSpec{Target: caller.Name, Task: "x"}); err == nil || !strings.Contains(err.Error(), "already part of this chain") {
			t.Fatalf("expected cycle guard, got %v", err)
		}
	})

	t.Run("unknown target", func(t *testing.T) {
		var n int32
		run := runAgentFor(t, rt, caller, delegState{depth: 0, visited: map[string]bool{caller.ID: true}, calls: &n})
		if _, err := run(tools.RunAgentSpec{Target: "Ghost", Task: "x"}); err == nil || !strings.Contains(err.Error(), "unknown subagent target") {
			t.Fatalf("expected unknown-target error, got %v", err)
		}
	})
}

// TestResolveSubagentProfile verifies a profile target yields an ephemeral agent
// cloned from the caller but reshaped with the profile's prompt and allowlist,
// while an unknown target falls through to agent resolution.
func TestResolveSubagentProfile(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	caller := db.Agent{ID: "AGT1", Name: "Caller", Provider: "anthropic", Model: "claude", PermissionMode: "ask"}

	eph, ephemeral, err := rt.resolveSubagentTarget(context.Background(), caller, "explore")
	if err != nil || !ephemeral {
		t.Fatalf("expected ephemeral explore profile, got ephemeral=%v err=%v", ephemeral, err)
	}
	if eph.ID != caller.ID || eph.Provider != caller.Provider || eph.Model != caller.Model {
		t.Fatalf("ephemeral agent must inherit caller infra: %+v", eph)
	}
	if eph.Soul == "" || eph.Name != "subagent:explore" {
		t.Fatalf("ephemeral agent must carry the profile persona: %+v", eph)
	}
	var allow []string
	_ = json.Unmarshal([]byte(eph.AllowedTools), &allow)
	if len(allow) == 0 {
		t.Fatalf("explore profile must set an allowlist, got %q", eph.AllowedTools)
	}
}

// TestDelegationContract verifies the structured task contract renders only set
// fields and stays empty (back-compat) when no field is provided.
func TestDelegationContract(t *testing.T) {
	if got := delegationContract(tools.RunAgentSpec{Task: "do x"}); got != "" {
		t.Fatalf("empty contract expected for a plain task, got %q", got)
	}

	full := delegationContract(tools.RunAgentSpec{
		Objective:    "find the bug",
		OutputFormat: "bulleted list",
		Boundaries:   "internal/db only",
	})
	for _, want := range []string{
		"## Task contract",
		"- Objective: find the bug",
		"- Output format: bulleted list",
		"- Boundaries (do NOT exceed): internal/db only",
	} {
		if !strings.Contains(full, want) {
			t.Fatalf("contract missing %q in:\n%s", want, full)
		}
	}

	// A single field renders that line only — the others must not appear.
	one := delegationContract(tools.RunAgentSpec{OutputFormat: "JSON"})
	if !strings.Contains(one, "- Output format: JSON") {
		t.Fatalf("expected output-format line, got %q", one)
	}
	if strings.Contains(one, "Objective:") || strings.Contains(one, "Boundaries") {
		t.Fatalf("unset fields must be omitted, got %q", one)
	}
}
