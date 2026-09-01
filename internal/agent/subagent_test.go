package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/tools"
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

// TestResolveSubagentAgentBeatsProfile verifies an existing workspace agent wins
// over a built-in profile of the same name (case-insensitive). Regression for the
// shadowing bug where a real "Reviewer" agent was intercepted by the `reviewer`
// profile.
func TestResolveSubagentAgentBeatsProfile(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	ctx := context.Background()
	caller, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "Caller", Provider: "anthropic"})
	real, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "Reviewer", Provider: "anthropic"})

	got, ephemeral, err := rt.resolveSubagentTarget(ctx, caller, "Reviewer")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if ephemeral || got.ID != real.ID {
		t.Fatalf("real agent must win over the %q profile: ephemeral=%v id=%q (want %q)", "reviewer", ephemeral, got.ID, real.ID)
	}

	// With no real agent of that name, the profile still resolves (ephemeral).
	_, ephemeral2, err := rt.resolveSubagentTarget(ctx, caller, "reviewer2-not-an-agent")
	if err == nil || ephemeral2 {
		t.Fatalf("unknown non-profile target must error, got ephemeral=%v err=%v", ephemeral2, err)
	}
	if p, ep, perr := rt.resolveSubagentTarget(ctx, caller, "coder"); perr != nil || !ep || p.Name != "subagent:coder" {
		t.Fatalf("coder profile must resolve ephemeral when no such agent exists: %+v ep=%v err=%v", p, ep, perr)
	}
}

func TestResolveSubagentExactProfileBeatsSameNamedAgent(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	ctx := context.Background()
	caller, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "Caller", Provider: "codex-cli"})
	_, _ = rt.db.CreateAgent(ctx, db.Agent{Name: "validator", Provider: "codex-cli", AllowedTools: `[]`})

	got, ephemeral, err := rt.resolveSubagentTarget(ctx, caller, "validator")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !ephemeral || got.Name != "subagent:validator" {
		t.Fatalf("exact profile id was shadowed by persisted agent: ephemeral=%v agent=%+v", ephemeral, got)
	}
	if got.AllowedTools != mustJSON(t, defaultSubagentProfiles["validator"].AllowedTools) {
		t.Fatalf("validator profile grants not applied: %s", got.AllowedTools)
	}
}

// TestResolveSubagentConfigProfile verifies the config mini-agent profile resolves
// to an ephemeral worker whose allowlist is restricted to the config tools, so it
// can only touch config/ and never wanders the filesystem.
func TestResolveSubagentConfigProfile(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	caller := db.Agent{ID: "AGT1", Name: "Caller", Provider: "anthropic", Model: "claude", PermissionMode: "ask"}

	eph, ephemeral, err := rt.resolveSubagentTarget(context.Background(), caller, "config")
	if err != nil || !ephemeral {
		t.Fatalf("expected ephemeral config profile, got ephemeral=%v err=%v", ephemeral, err)
	}
	if eph.Name != "subagent:config" || eph.Soul == "" {
		t.Fatalf("config profile must carry its persona: %+v", eph)
	}
	var allow []string
	_ = json.Unmarshal([]byte(eph.AllowedTools), &allow)
	want := map[string]bool{"list_config": true, "read_config": true, "write_config": true, "config_validate": true}
	if len(allow) != len(want) {
		t.Fatalf("config profile allowlist = %v, want the 4 config tools", allow)
	}
	for _, tool := range allow {
		if !want[tool] {
			t.Fatalf("config profile must not allow %q (config-only sandbox); allowlist=%v", tool, allow)
		}
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

// A bound sink publishes a partial, Running card on every nested step, so the
// chat shows the delegation while it is still in flight instead of staying
// empty until the subagent's final reply lands.
func TestSubStepSinkEmitsLiveCards(t *testing.T) {
	var got []TurnStep
	s := &subStepSink{}
	card := openLive(func(st TurnStep) { got = append(got, st) }, "call-1", TurnStep{Kind: StepTool})
	s.bindLive(card)

	s.emitLive(nil) // start card, before any nested step
	s.addSteps(TurnStep{Kind: StepTool, Tool: "Read"})
	s.emitLive(nil)

	if len(got) != 3 {
		t.Fatalf("want open card + 2 live updates, got %d", len(got))
	}
	for i, st := range got[1:] {
		if st.Kind != StepSubagent || st.ID != "call-1" || !st.Running {
			t.Fatalf("card %d is not a running subagent card for call-1: %+v", i, st)
		}
	}
	if len(got[1].SubSteps) != 0 || len(got[2].SubSteps) != 1 {
		t.Fatalf("live cards must carry the steps gathered so far: %d then %d",
			len(got[1].SubSteps), len(got[2].SubSteps))
	}
	if len(s.collected()) != 1 {
		t.Fatalf("sink must keep collecting for the final card, got %d", len(s.collected()))
	}
}

// An unbound sink keeps the legacy collect-only behaviour.
func TestSubStepSinkWithoutLiveEmitterIsSilent(t *testing.T) {
	s := &subStepSink{}
	s.emitLive(nil) // must not panic
	s.addSteps(TurnStep{Kind: StepTool, Tool: "Read"})
	if len(s.collected()) != 1 {
		t.Fatalf("want 1 collected step, got %d", len(s.collected()))
	}
}
