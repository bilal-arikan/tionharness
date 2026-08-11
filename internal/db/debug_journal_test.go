package db

import (
	"context"
	"math"
	"path/filepath"
	"testing"
)

// TestDebugJournalRoundTrip verifies append → read-back, type filtering and the
// aggregate summary (turns, llm-call tokens by model, per-tool rollups, errors).
func TestDebugJournalRoundTrip(t *testing.T) {
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	agent, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	sess, _ := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "T"})

	events := []DebugEvent{
		{Type: DebugLLMCall, Model: "claude", In: 100, Out: 20, CacheRead: 50},
		{Type: DebugTool, Name: "Bash", DurMs: 120, OutBytes: 400},
		{Type: DebugTool, Name: "Bash", DurMs: 80, OutBytes: 200, Err: true},
		{Type: DebugTool, Name: "Read", DurMs: 10, OutBytes: 4000},
		{Type: DebugError, Detail: "boom"},
		{Type: DebugCompaction, SavedBytes: 1234},
		{Type: DebugTurn, DurMs: 500, Stop: "end_turn"},
	}
	for _, e := range events {
		if err := d.AppendDebugEvent(sess.ID, e, 0); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	all, err := d.ReadDebugEvents(ctx, sess.ID, "", 0)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(all) != len(events) {
		t.Fatalf("read count = %d, want %d", len(all), len(events))
	}
	if all[0].Time == 0 {
		t.Errorf("append did not stamp Time")
	}

	tools, _ := d.ReadDebugEvents(ctx, sess.ID, DebugTool, 0)
	if len(tools) != 3 {
		t.Fatalf("tool events = %d, want 3", len(tools))
	}

	sum, err := d.GetDebugSummary(ctx, sess.ID)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if sum.Turns != 1 || sum.LLMCalls != 1 || sum.ToolCalls != 3 {
		t.Errorf("counts: turns=%d llm=%d tools=%d", sum.Turns, sum.LLMCalls, sum.ToolCalls)
	}
	if sum.InputTokens != 100 || sum.OutputTokens != 20 || sum.CacheRead != 50 {
		t.Errorf("tokens: in=%d out=%d cache=%d", sum.InputTokens, sum.OutputTokens, sum.CacheRead)
	}
	if sum.ByModel["claude"] != 120 {
		t.Errorf("byModel[claude] = %d, want 120", sum.ByModel["claude"])
	}
	bash := sum.ByTool["Bash"]
	if bash.Calls != 2 || bash.Errors != 1 || bash.DurMs != 200 {
		t.Errorf("Bash stat = %+v", bash)
	}
	if sum.Errors != 1 || sum.LastError != "boom" {
		t.Errorf("errors=%d last=%q", sum.Errors, sum.LastError)
	}
	if sum.Compactions != 1 || sum.SavedBytes != 1234 {
		t.Errorf("compactions=%d saved=%d", sum.Compactions, sum.SavedBytes)
	}
	if len(sum.TopTools) == 0 || sum.TopTools[0] != "Bash" {
		t.Errorf("topTools = %v, want Bash first (slowest)", sum.TopTools)
	}
}

// TestDebugSummaryAnomaliesAndSeries verifies the Faz-3 derived fields: the
// per-turn / per-call time series and the heuristic anomaly findings.
func TestDebugSummaryAnomaliesAndSeries(t *testing.T) {
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	agent, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	sess, _ := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "T"})

	emit := func(e DebugEvent) {
		if err := d.AppendDebugEvent(sess.ID, e, 0); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	// One slow dominant tool (Bash ~95% of tool time, > 2s total).
	emit(DebugEvent{Type: DebugTool, Name: "Bash", DurMs: 5000, OutBytes: 100})
	emit(DebugEvent{Type: DebugTool, Name: "Read", DurMs: 50, OutBytes: 100})
	// A failing tool (calls=3, 2 errors → >30%).
	for i := 0; i < 3; i++ {
		emit(DebugEvent{Type: DebugTool, Name: "Flaky", DurMs: 10, Err: i < 2})
	}
	// Frequent compaction (>=3) + error burst (>=3).
	for i := 0; i < 3; i++ {
		emit(DebugEvent{Type: DebugCompaction})
		emit(DebugEvent{Type: DebugError, Detail: "boom"})
	}
	// Time series points.
	emit(DebugEvent{Type: DebugTurn, DurMs: 100})
	emit(DebugEvent{Type: DebugTurn, DurMs: 200})
	emit(DebugEvent{Type: DebugLLMCall, Model: "m", In: 10, Out: 5, Think: 2})
	emit(DebugEvent{Type: DebugLLMCall, Model: "m", In: 20, Out: 5, Think: 3})

	sum, err := d.GetDebugSummary(ctx, sess.ID)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	// Thinking attribution: summed (2+3) and its share of total output (5/10).
	if sum.ThinkingTokens != 5 {
		t.Errorf("thinkingTokens = %d, want 5", sum.ThinkingTokens)
	}
	if sum.ThinkingShare != 0.5 {
		t.Errorf("thinkingShare = %v, want 0.5", sum.ThinkingShare)
	}
	if len(sum.TurnDurSeries) != 2 || sum.TurnDurSeries[1] != 200 {
		t.Errorf("turnDurSeries = %v", sum.TurnDurSeries)
	}
	if len(sum.TokenSeries) != 2 || sum.TokenSeries[1] != 25 {
		t.Errorf("tokenSeries = %v", sum.TokenSeries)
	}
	codes := map[string]bool{}
	for _, a := range sum.Anomalies {
		codes[a.Code] = true
	}
	for _, want := range []string{"tool_time_dominant", "tool_failing", "frequent_compaction", "error_burst"} {
		if !codes[want] {
			t.Errorf("missing anomaly %q; got %v", want, codes)
		}
	}
}

// TestDebugSummaryCacheBreaks verifies cache_break events are counted, the last
// reason is captured, and the anomaly escalates from info (1) to warn (>=2).
func TestDebugSummaryCacheBreaks(t *testing.T) {
	ctx := context.Background()
	d, _ := Open(filepath.Join(t.TempDir(), "store"))
	agent, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})

	// One break → info anomaly, reason captured.
	s1, _ := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "one"})
	_ = d.AppendDebugEvent(s1.ID, DebugEvent{Type: DebugCacheBreak, Name: "ttl-or-server-eviction", Detail: "TTL doldu"}, 0)
	sum1, _ := d.GetDebugSummary(ctx, s1.ID)
	if sum1.CacheBreaks != 1 || sum1.LastCacheBreak != "TTL doldu" {
		t.Fatalf("cacheBreaks=%d last=%q, want 1 / TTL doldu", sum1.CacheBreaks, sum1.LastCacheBreak)
	}
	if code := anomalyCode(sum1.Anomalies, "cache_break"); code == nil || code.Severity != "info" {
		t.Errorf("one break should raise an info cache_break anomaly, got %+v", sum1.Anomalies)
	}

	// Two breaks → warn anomaly.
	s2, _ := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "two"})
	_ = d.AppendDebugEvent(s2.ID, DebugEvent{Type: DebugCacheBreak, Detail: "model değişti"}, 0)
	_ = d.AppendDebugEvent(s2.ID, DebugEvent{Type: DebugCacheBreak, Detail: "prompt değişti"}, 0)
	sum2, _ := d.GetDebugSummary(ctx, s2.ID)
	if sum2.CacheBreaks != 2 {
		t.Fatalf("cacheBreaks=%d, want 2", sum2.CacheBreaks)
	}
	if code := anomalyCode(sum2.Anomalies, "cache_breaks"); code == nil || code.Severity != "warn" {
		t.Errorf("two breaks should raise a warn cache_breaks anomaly, got %+v", sum2.Anomalies)
	}
}

// TestDebugSummaryCoolingWaste verifies the isolated cooling-waste rollup: only
// cache_break events carrying a WasteUSD contribute, they are summed, counted,
// and the estimated flag latches when any contributing figure is an estimate.
func TestDebugSummaryCoolingWaste(t *testing.T) {
	ctx := context.Background()
	d, _ := Open(filepath.Join(t.TempDir(), "store"))
	agent, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	s, _ := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "cool"})

	// Two TTL breaks with waste (one estimated) + a model-change break with none.
	_ = d.AppendDebugEvent(s.ID, DebugEvent{Type: DebugCacheBreak, Name: "ttl-or-server-eviction", WasteUSD: 0.02}, 0)
	_ = d.AppendDebugEvent(s.ID, DebugEvent{Type: DebugCacheBreak, Name: "ttl-or-server-eviction", WasteUSD: 0.03, WasteEstimated: true}, 0)
	_ = d.AppendDebugEvent(s.ID, DebugEvent{Type: DebugCacheBreak, Name: "model-changed", Detail: "model değişti"}, 0)

	sum, _ := d.GetDebugSummary(ctx, s.ID)
	if sum.CacheBreaks != 3 {
		t.Fatalf("cacheBreaks=%d, want 3", sum.CacheBreaks)
	}
	if sum.CoolingBreaks != 2 {
		t.Errorf("coolingBreaks=%d, want 2 (only waste-bearing breaks)", sum.CoolingBreaks)
	}
	if math.Abs(sum.CoolingWasteUSD-0.05) > 1e-9 {
		t.Errorf("coolingWasteUsd=%v, want 0.05", sum.CoolingWasteUSD)
	}
	if !sum.CoolingWasteEstimated {
		t.Error("coolingWasteEstimated should latch true when any figure is estimated")
	}
}

// TestAddCoolingWaste verifies cooling-waste USD accumulates into today's usage
// row (upsert), latches the estimated flag, and ignores non-positive amounts.
func TestAddCoolingWaste(t *testing.T) {
	ctx := context.Background()
	d, _ := Open(filepath.Join(t.TempDir(), "store"))
	agent, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})

	if err := d.AddCoolingWaste(ctx, agent.ID, 0, false); err != nil {
		t.Fatalf("zero waste err: %v", err)
	}
	_ = d.AddCoolingWaste(ctx, agent.ID, 0.02, false)
	_ = d.AddCoolingWaste(ctx, agent.ID, 0.03, true) // estimate → latches flag

	u, _ := d.GetUsageToday(ctx, agent.ID)
	if math.Abs(u.CoolingWasteUSD-0.05) > 1e-9 {
		t.Errorf("coolingWasteUsd=%v, want 0.05", u.CoolingWasteUSD)
	}
	if !u.CoolingWasteEstimated {
		t.Error("estimated flag should latch true after an estimated contribution")
	}
}

// TestDebugSummaryCacheAndThinkingCoach verifies the two observability coaches:
// a sustained low warm-hit ratio raises low_cache_hit, and a large hidden-
// reasoning share of a meaningful output spend raises high_thinking.
func TestDebugSummaryCacheAndThinkingCoach(t *testing.T) {
	ctx := context.Background()
	d, _ := Open(filepath.Join(t.TempDir(), "store"))
	agent, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})

	// 3 calls, big prompt spend, almost no cache reads → low hit ratio.
	// Output carries a majority-thinking share too.
	s, _ := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "coach"})
	for i := 0; i < 3; i++ {
		_ = d.AppendDebugEvent(s.ID, DebugEvent{
			Type: DebugLLMCall, Model: "m", In: 10000, Out: 1000, Think: 700, CacheRead: 500,
		}, 0)
	}
	sum, _ := d.GetDebugSummary(ctx, s.ID)
	if code := anomalyCode(sum.Anomalies, "low_cache_hit"); code == nil || code.Severity != "warn" {
		t.Errorf("low warm-hit ratio should raise a warn low_cache_hit anomaly, got %+v", sum.Anomalies)
	}
	if code := anomalyCode(sum.Anomalies, "high_thinking"); code == nil || code.Severity != "info" {
		t.Errorf("majority-thinking output should raise an info high_thinking anomaly, got %+v", sum.Anomalies)
	}

	// Warm session: high cache reads → no low_cache_hit.
	s2, _ := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "warm"})
	for i := 0; i < 3; i++ {
		_ = d.AppendDebugEvent(s2.ID, DebugEvent{Type: DebugLLMCall, Model: "m", In: 500, Out: 200, CacheRead: 40000}, 0)
	}
	sum2, _ := d.GetDebugSummary(ctx, s2.ID)
	if code := anomalyCode(sum2.Anomalies, "low_cache_hit"); code != nil {
		t.Errorf("warm cache should not flag low_cache_hit, got %+v", sum2.Anomalies)
	}
}

// anomalyCode returns the first anomaly with the given code, or nil.
func anomalyCode(as []DebugAnomaly, code string) *DebugAnomaly {
	for i := range as {
		if as[i].Code == code {
			return &as[i]
		}
	}
	return nil
}

// TestDebugSummaryNoAnomaliesOnHealthy verifies a healthy session is quiet.
func TestDebugSummaryNoAnomaliesOnHealthy(t *testing.T) {
	ctx := context.Background()
	d, _ := Open(filepath.Join(t.TempDir(), "store"))
	agent, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	sess, _ := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "T"})
	_ = d.AppendDebugEvent(sess.ID, DebugEvent{Type: DebugTool, Name: "Read", DurMs: 20, OutBytes: 100}, 0)
	_ = d.AppendDebugEvent(sess.ID, DebugEvent{Type: DebugTurn, DurMs: 500}, 0)
	sum, _ := d.GetDebugSummary(ctx, sess.ID)
	if len(sum.Anomalies) != 0 {
		t.Errorf("healthy session flagged anomalies: %v", sum.Anomalies)
	}
}

// TestDebugJournalCapPrunes verifies the file is pruned to the newest cap events
// once it grows past cap + cap/4, and that the survivors are the newest ones.
func TestDebugJournalCapPrunes(t *testing.T) {
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	agent, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	sess, _ := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "T"})

	cap := 8
	// Append well past cap + cap/4 so pruning triggers at least once.
	for i := 0; i < 40; i++ {
		if err := d.AppendDebugEvent(sess.ID, DebugEvent{Type: DebugTool, Name: "T", DurMs: int64(i)}, cap); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	evs, err := d.ReadDebugEvents(ctx, sess.ID, "", 0)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(evs) > cap+cap/4 {
		t.Fatalf("retained %d events, want <= %d", len(evs), cap+cap/4)
	}
	// The newest event (DurMs=39) must survive; the oldest (DurMs=0) must be gone.
	last := evs[len(evs)-1]
	if last.DurMs != 39 {
		t.Errorf("newest survivor DurMs = %d, want 39", last.DurMs)
	}
}

// TestGetTurnDebugCacheBreak verifies the PER-MESSAGE rollup attributes a cache
// break to the turn that paid for it: the reason/detail travel with it, the
// avoidable cooling overpay is summed with its estimated flag, and a break tagged
// to a DIFFERENT turn never leaks into this one.
func TestGetTurnDebugCacheBreak(t *testing.T) {
	ctx := context.Background()
	d, _ := Open(filepath.Join(t.TempDir(), "store"))
	agent, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	s, _ := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "t"})

	_ = d.AppendDebugEvent(s.ID, DebugEvent{
		Type: DebugLLMCall, TurnID: "MSG1", Model: "opus", In: 12, Out: 3, CacheWrite: 9000,
	}, 0)
	_ = d.AppendDebugEvent(s.ID, DebugEvent{
		Type: DebugCacheBreak, TurnID: "MSG1", Name: "ttl-or-server-eviction",
		Detail: "Önek değişmedi ama cache okunmadı", WasteUSD: 0.021, WasteEstimated: true,
	}, 0)
	// A second turn's break must stay out of MSG1's rollup.
	_ = d.AppendDebugEvent(s.ID, DebugEvent{
		Type: DebugCacheBreak, TurnID: "MSG2", Name: "model-changed", Detail: "Model değişti",
	}, 0)

	td, err := d.GetTurnDebug(ctx, s.ID, "MSG1")
	if err != nil {
		t.Fatalf("GetTurnDebug: %v", err)
	}
	if td.CacheBreaks != 1 {
		t.Fatalf("cacheBreaks=%d, want 1 (MSG2's break must not leak)", td.CacheBreaks)
	}
	if td.CacheBreakReason != "ttl-or-server-eviction" || td.CacheBreakDetail == "" {
		t.Errorf("reason=%q detail=%q, want the ttl tag with its detail", td.CacheBreakReason, td.CacheBreakDetail)
	}
	if td.CoolingWasteUSD != 0.021 || !td.CoolingWasteEstimated {
		t.Errorf("waste=%v est=%v, want 0.021 / true", td.CoolingWasteUSD, td.CoolingWasteEstimated)
	}

	// A turn with no break reports none — the zero value, so the panel renders nothing.
	td2, _ := d.GetTurnDebug(ctx, s.ID, "MSG3")
	if td2.CacheBreaks != 0 || td2.CacheBreakReason != "" {
		t.Errorf("unknown turn should carry no break, got %+v", td2)
	}
}
