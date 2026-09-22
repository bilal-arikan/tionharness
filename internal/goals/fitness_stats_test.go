package goals

import (
	"fmt"
	"math"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestDistributionOf(t *testing.T) {
	if _, ok := distributionOf(nil); ok {
		t.Fatal("empty sample must report ok=false")
	}
	// 10 samples: 1..9 and one outlier 100. Trim cuts 1 from each end.
	s := []float64{100, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	d, ok := distributionOf(s)
	if !ok || d.N != 10 {
		t.Fatalf("n = %d ok=%v", d.N, ok)
	}
	if !near(d.Mean, 14.5) {
		t.Errorf("mean = %v, want 14.5", d.Mean)
	}
	if !near(d.Median, 5.5) {
		t.Errorf("median = %v, want 5.5", d.Median)
	}
	if !near(d.TrimmedMean, 5.5) { // 2..9 → 44/8
		t.Errorf("trimmed mean = %v, want 5.5", d.TrimmedMean)
	}
	if !near(d.Top3Share, (100.0+9+8)/145) {
		t.Errorf("top3 share = %v", d.Top3Share)
	}
	if s[0] != 100 {
		t.Error("distributionOf must not reorder the caller's slice")
	}
	odd, _ := distributionOf([]float64{3, 1, 2})
	if odd.Median != 2 || odd.TrimmedMean != 2 || !near(odd.Top3Share, 1) {
		t.Errorf("odd sample = %+v", odd)
	}
}

// bucketFixture builds n costed sessions under one snapshot, the first
// `heavy` of them costing heavyCost and the rest 1 USD, each with 10 tool calls.
func bucketFixture(n, heavy int, heavyCost float64) ([]SessionRow, FitnessInputs) {
	in := FitnessInputs{Usage: map[string]UsageRow{}}
	var rows []SessionRow
	for i := range n {
		id := fmt.Sprintf("SES%d", i)
		cost := 1.0
		if i < heavy {
			cost = heavyCost
		}
		rows = append(rows, SessionRow{ID: id, AgentID: "AGT1", SnapshotHash: "h", ToolCalls: 10})
		in.Usage[id] = UsageRow{Tokens: 100, CostUSD: cost}
	}
	return rows, in
}

func TestBucketStatsGate(t *testing.T) {
	// Small bucket: gated on n.
	rows, in := bucketFixture(MinBucketSessions-1, 0, 0)
	st := bucketStats(rows, in)
	if !st.Insufficient || len(st.Reasons) != 1 {
		t.Fatalf("n below minimum must gate: %+v", st)
	}

	// Large, even bucket: passes the gate; cost per tool call = 1/10.
	rows, in = bucketFixture(MinBucketSessions+10, 0, 0)
	st = bucketStats(rows, in)
	if st.Insufficient {
		t.Fatalf("large even bucket must not gate: %+v", st)
	}
	if st.CostPerToolCall == nil || !near(*st.CostPerToolCall, 0.1) {
		t.Fatalf("cost per tool call = %v", st.CostPerToolCall)
	}
	if st.ToolCalls != (MinBucketSessions+10)*10 {
		t.Errorf("tool calls = %d", st.ToolCalls)
	}

	// Large but dominated by three heavy sessions: gated on top-3 share.
	rows, in = bucketFixture(MinBucketSessions+10, 3, 100)
	st = bucketStats(rows, in)
	if !st.Insufficient || st.Cost.Top3Share <= MaxTop3CostShare {
		t.Fatalf("skewed bucket must gate: %+v", st)
	}
	if !near(st.Cost.Median, 1) {
		t.Errorf("median must ignore the outliers, got %v", st.Cost.Median)
	}

	// Zero-token sessions are not samples.
	rows = append(rows, SessionRow{ID: "SESZ", RunState: "failed"})
	if got := bucketStats(rows, in).Cost.N; got != MinBucketSessions+10 {
		t.Errorf("zero-token session counted: n = %d", got)
	}
}

func TestErrorRatioExcludesProviderFailures(t *testing.T) {
	g := db.Goal{ID: "GOL1", Primary: db.GoalMetric{Metric: "usage.costUSDPerSession", Direction: "min"},
		Guardrails: []db.GoalGuardrail{{Metric: "session.errorTurnsRatio", Max: new(0.10)}}}
	rows, in := bucketFixture(20, 0, 0)
	// Three sessions that failed on login before any token was spent, one
	// real failure (spent tokens) and one failed with tool calls but no
	// usage row (still real work — counts).
	rows = append(rows,
		SessionRow{ID: "AUTH1", AgentID: "AGT2", SnapshotHash: "h", RunState: "failed"},
		SessionRow{ID: "AUTH2", AgentID: "AGT2", SnapshotHash: "h", RunState: "failed"},
		SessionRow{ID: "AUTH3", AgentID: "AGT2", SnapshotHash: "h", RunState: "error"},
		SessionRow{ID: "REAL1", AgentID: "AGT1", SnapshotHash: "h", RunState: "failed", ToolCalls: 4},
	)
	in.Usage["AUTH3"] = UsageRow{Tokens: 0}
	in.Usage["REAL1"] = UsageRow{Tokens: 50, CostUSD: 0.2}
	in.Sessions = rows
	in.CurrentHash = "h"

	fit := Evaluate(g, in)
	gr := fit.Guardrails[0]
	if gr.Excluded != 3 || gr.N != 21 {
		t.Fatalf("excluded=%d n=%d, want 3/21", gr.Excluded, gr.N)
	}
	if gr.Value == nil || !near(*gr.Value, 1.0/21) || gr.Violated {
		t.Fatalf("ratio = %v violated=%v, want 1/21 not violated", gr.Value, gr.Violated)
	}
	if gr.Note == "" {
		t.Error("excluded failures must be surfaced in the note, not dropped silently")
	}
	if fit.ProviderFailures != 3 || fit.BySnapshot[0].ProviderFailures != 3 {
		t.Errorf("provider failures = %d / %d, want 3", fit.ProviderFailures, fit.BySnapshot[0].ProviderFailures)
	}
	if fit.Primary.Dist == nil || !near(fit.Primary.Dist.Median, 1) {
		t.Errorf("primary distribution missing or wrong: %+v", fit.Primary.Dist)
	}

	// Same-agent comparison: only AGT2's sessions; agents list stays complete.
	in.Agent = "AGT2"
	fit = Evaluate(g, in)
	if fit.Sessions != 3 || fit.Agent != "AGT2" || len(fit.Agents) != 2 || fit.Agents[0].AgentID != "AGT1" {
		t.Fatalf("agent filter: sessions=%d agents=%+v", fit.Sessions, fit.Agents)
	}
}
