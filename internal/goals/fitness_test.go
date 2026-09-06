package goals

import (
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func fitnessFixture() (db.Goal, FitnessInputs) {
	g := db.Goal{
		ID: "GOL1", Status: db.GoalStatusActive,
		Scope:      db.GoalScope{Recipes: []string{"code-review"}},
		Primary:    db.GoalMetric{Metric: "recipe.avgCostUSD", Direction: "min", Target: f(1)},
		Guardrails: []db.GoalGuardrail{{Metric: "recipe.successRate", Min: f(0.9)}, {Metric: "usage.tokensPerSession", Max: f(5000)}},
	}
	sum := func(cost float64, sessions int) *db.TrajectorySummary {
		return &db.TrajectorySummary{DurationSec: 100, Tokens: 1000, CostUSD: cost, Sessions: sessions, GateWaitSec: 10, GhostPhases: []string{"ship"}}
	}
	in := FitnessInputs{
		Now: 200_000, Since: 0,
		Sessions: []SessionRow{
			{ID: "SES1", RootID: "SES1", AgentID: "AGT1", SnapshotHash: "aaa", CreatedAt: 10_000, Tags: []string{"review"}},
			{ID: "SES2", RootID: "SES1", AgentID: "AGT2", SnapshotHash: "aaa", CreatedAt: 10_100, StuckTurns: 2},
			{ID: "SES3", RootID: "SES3", AgentID: "AGT1", SnapshotHash: "bbb", CreatedAt: 100_000, RunState: "failed"},
			{ID: "SES4", RootID: "SES4", AgentID: "AGT1", SnapshotHash: "bbb", CreatedAt: 100_500},
			{ID: "SES9", RootID: "SES9", AgentID: "AGT1", SnapshotHash: "bbb", CreatedAt: 100_600}, // other recipe → out of scope
			{ID: "SESX", RootID: "SESX", AgentID: "AGT1", SnapshotHash: "bbb", CreatedAt: 100_700, Kind: db.SessionKindInsight},
		},
		Trajectories: []db.TrajectoryIndexEntry{
			{ID: "RTA1", RootSessionID: "SES1", TemplateRef: "code-review@1", Status: db.TrajStatusDone, Summary: sum(2, 2)},
			{ID: "RTA2", RootSessionID: "SES3", TemplateRef: "code-review@2", Status: db.TrajStatusFailed, Summary: sum(0.5, 1)},
			{ID: "RTA3", RootSessionID: "SES4", TemplateRef: "code-review@2", Status: db.TrajStatusDone, Summary: sum(0.7, 1)},
			{ID: "RTA4", RootSessionID: "SES9", TemplateRef: "plan-dev@1", Status: db.TrajStatusDone, Summary: sum(9, 9)},
			{ID: "RTA5", RootSessionID: "SES4", TemplateRef: "code-review@2", Status: db.TrajStatusRunning},
		},
		Usage: map[string]UsageRow{
			"SES1": {Tokens: 4000, InputTokens: 3000, CacheRead: 1000, CostUSD: 1},
			"SES3": {Tokens: 8000, InputTokens: 8000, CostUSD: 2},
			"SES9": {Tokens: 99_000, CostUSD: 50},
		},
		Tasks: []TaskRow{
			{ID: "TSK1", BoardState: db.BoardDone, CreatedAt: 10_000, UpdatedAt: 12_000, SessionIDs: []string{"SES1"}},
			{ID: "TSK2", BoardState: db.BoardDone, CreatedAt: 100_000, UpdatedAt: 101_000, SessionIDs: []string{"SES9"}},
			{ID: "TSK3", BoardState: "todo", CreatedAt: 1, UpdatedAt: 2},
		},
		Fires: []FireRow{{AutomationID: "AUT1", At: 10_050, SessionID: "SES2"}, {AutomationID: "AUT1", At: 100_050, SessionID: "SES3", Failed: true}},
		Asks:  []AskRow{{SessionID: "SES1"}, {SessionID: "SES1"}, {SessionID: "SES9"}},
		Snapshots: map[string]ConfigSnapshot{
			"aaa": {Hash: "aaa", Agents: map[string]AgentGenome{"AGT1": {SoulHash: "s1", SoulChars: 100, Skills: []string{"a"}}, "AGT2": {SoulChars: 300}}, Recipes: map[string]string{"code-review": "1"}},
			"bbb": {Hash: "bbb", Agents: map[string]AgentGenome{"AGT1": {SoulHash: "s2", SoulChars: 150, Skills: []string{"a", "b"}}, "AGT2": {SoulChars: 300}}, Recipes: map[string]string{"code-review": "2"}},
		},
		CurrentHash: "bbb",
	}
	return g, in
}

func val(mv MetricValue) float64 {
	if mv.Value == nil {
		return -1
	}
	return *mv.Value
}

// TestEvaluateRecipeScope: recipe scope keeps the trajectory roots and their
// children, drops insight sessions and other recipes; metrics are grouped by
// snapshot in first-seen order with diffs on the edges.
func TestEvaluateRecipeScope(t *testing.T) {
	g, in := fitnessFixture()
	out := Evaluate(g, in)
	if out.Sessions != 4 {
		t.Fatalf("scoped sessions = %d, want 4 (SES1,2,3,4)", out.Sessions)
	}
	// Whole window: 3 terminal runs, 2 done, costs 2+0.5+0.7 over 3 summarized.
	if got := val(out.Primary); got < 1.06 || got > 1.07 || out.Primary.N != 3 {
		t.Fatalf("primary = %v (n=%d), want ≈1.067 over 3", got, out.Primary.N)
	}
	if out.OnTarget == nil || *out.OnTarget {
		t.Fatal("1.067 > target 1 must be off target")
	}
	if got := val(out.Guardrails[0].MetricValue); got < 0.66 || got > 0.67 || !out.Guardrails[0].Violated {
		t.Fatalf("success rate = %v violated=%v", got, out.Guardrails[0].Violated)
	}
	// tokensPerSession over sessions with usage in scope (SES1 4000, SES3 8000).
	if got := val(out.Guardrails[1].MetricValue); got != 6000 || !out.Guardrails[1].Violated {
		t.Fatalf("tokens/session = %v", got)
	}
	if len(out.BySnapshot) != 2 || out.BySnapshot[0].Hash != "aaa" || out.BySnapshot[1].Hash != "bbb" || !out.BySnapshot[1].Current {
		t.Fatalf("snapshot series = %+v", out.BySnapshot)
	}
	if got := val(out.BySnapshot[0].Primary); got != 2 || out.BySnapshot[0].Sessions != 2 {
		t.Fatalf("aaa primary = %v sessions=%d", got, out.BySnapshot[0].Sessions)
	}
	if got := val(out.BySnapshot[1].Primary); got != 0.6 {
		t.Fatalf("bbb primary = %v, want 0.6", got)
	}
	if got := val(out.BySnapshot[1].Guardrails[0].MetricValue); got != 0.5 {
		t.Fatalf("bbb success = %v, want 0.5", got)
	}
	if len(out.BySnapshot[1].Changes) == 0 {
		t.Fatal("bbb must carry the diff against aaa")
	}
	seen := map[string]bool{}
	for _, c := range out.BySnapshot[1].Changes {
		seen[c.Surface+":"+c.Entity+":"+c.Field] = true
	}
	if !seen["recipe:code-review:version"] || !seen["agent:AGT1:soul"] || !seen["agent:AGT1:skills"] {
		t.Fatalf("diff missing expected entries: %v", seen)
	}
}

// TestEvaluateOtherMetrics covers the non-recipe evaluators over an unscoped goal.
func TestEvaluateOtherMetrics(t *testing.T) {
	_, in := fitnessFixture()
	all := db.Goal{ID: "G", Primary: db.GoalMetric{Metric: "session.errorTurnsRatio", Direction: "min"}}
	check := func(metric string, want float64, wantN int) {
		t.Helper()
		g := all
		g.Primary.Metric = metric
		out := Evaluate(g, in)
		if got := val(out.Primary); got != want || out.Primary.N != wantN {
			t.Fatalf("%s = %v (n=%d), want %v (n=%d)", metric, got, out.Primary.N, want, wantN)
		}
	}
	// 5 non-insight sessions; SES3 failed.
	check("session.errorTurnsRatio", 0.2, 5)
	check("session.stuckLoops", 0.4, 5)
	check("session.humanAsksPerSession", 0.6, 5) // 3 asks / 5
	check("usage.cacheHitRatio", 1000.0/(3000+1000+8000+0), 3)
	check("board.cycleTimeSec", 1500, 2) // (2000 + 1000) / 2
	check("automation.errorRate", 0.5, 2)
	check("config.soulChars", 225, 2) // current snapshot bbb: (150+300)/2
	check("config.skillCount", 1, 2)

	// Agent scope narrows config metrics and sessions; tag scope intersects.
	g := all
	g.Primary.Metric = "config.soulChars"
	g.Scope.Agents = []string{"AGT1"}
	if got := val(Evaluate(g, in).Primary); got != 150 {
		t.Fatalf("config.soulChars for AGT1 = %v", got)
	}
	g.Primary.Metric = "usage.costUSDPerSession"
	g.Scope.Tags = []string{"review"}
	out := Evaluate(g, in)
	if out.Sessions != 1 || val(out.Primary) != 1 {
		t.Fatalf("agent+tag scope: sessions=%d cost=%v", out.Sessions, val(out.Primary))
	}
	// Automation scope follows the fired sessions.
	g = all
	g.Scope.Automations = []string{"AUT1"}
	g.Primary.Metric = "automation.firesPerDay"
	out = Evaluate(g, in)
	if out.Sessions != 2 || out.Primary.N != 2 {
		t.Fatalf("automation scope: sessions=%d fires=%d", out.Sessions, out.Primary.N)
	}
	// Unavailable and unknown metrics say so instead of inventing a number.
	g = all
	g.Primary.Metric = "judge.rubricScore"
	if mv := Evaluate(g, in).Primary; mv.Available || mv.Value != nil {
		t.Fatalf("unavailable metric must have no value: %+v", mv)
	}
	// Window: since after the "aaa" sessions leaves only "bbb".
	g = all
	g.Primary.Metric = "session.errorTurnsRatio"
	in.Since = 50_000
	out = Evaluate(g, in)
	if out.Sessions != 3 || len(out.BySnapshot) != 1 {
		t.Fatalf("window: sessions=%d groups=%d", out.Sessions, len(out.BySnapshot))
	}
}

// TestSnapshotHashCanonical: hash ignores capture time and map/slice order,
// changes with content; Diff reports exactly what moved.
func TestSnapshotHashCanonical(t *testing.T) {
	a := ConfigSnapshot{At: 1, Agents: map[string]AgentGenome{"AGT1": {Skills: []string{"b", "a"}, Model: "sonnet"}}, Tools: ToolsGenome{Disabled: []string{"z", "y"}}}
	b := ConfigSnapshot{At: 2, Agents: map[string]AgentGenome{"AGT1": {Skills: []string{"a", "b"}, Model: "sonnet"}}, Tools: ToolsGenome{Disabled: []string{"y", "z"}}}
	if err := a.Finalize(); err != nil {
		t.Fatal(err)
	}
	if err := b.Finalize(); err != nil {
		t.Fatal(err)
	}
	if a.Hash == "" || a.Hash != b.Hash {
		t.Fatalf("equal content must hash equal: %s vs %s", a.Hash, b.Hash)
	}
	c := b
	c.Agents = map[string]AgentGenome{"AGT1": {Skills: []string{"a", "b"}, Model: "opus"}}
	_ = c.Finalize()
	if c.Hash == b.Hash {
		t.Fatal("model change must change the hash")
	}
	d := Diff(b, c)
	if len(d) != 1 || d[0].Surface != "agent" || d[0].Field != "model" || d[0].Before != "sonnet" || d[0].After != "opus" {
		t.Fatalf("diff = %+v", d)
	}
	if len(Diff(a, b)) != 0 {
		t.Fatal("equal snapshots must not differ")
	}
	e := c
	e.Models = map[string]string{"claude-cli|opus": "claude-opus-5"}
	e.Automations = map[string]AutomationGenome{"AUT1": {Name: "n"}}
	d = Diff(c, e)
	if len(d) != 2 {
		t.Fatalf("diff = %+v", d)
	}
}
