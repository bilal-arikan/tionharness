package db

import (
	"context"
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
	emit(DebugEvent{Type: DebugLLMCall, Model: "m", In: 10, Out: 5})
	emit(DebugEvent{Type: DebugLLMCall, Model: "m", In: 20, Out: 5})

	sum, err := d.GetDebugSummary(ctx, sess.ID)
	if err != nil {
		t.Fatalf("summary: %v", err)
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
