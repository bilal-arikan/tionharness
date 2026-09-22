package decider

import (
	"testing"
	"time"
)

func TestLedgerStatsAndReload(t *testing.T) {
	dir := t.TempDir()
	l := OpenLedger(dir, nil)
	now := time.Now().UnixMilli()
	recs := []Record{
		{At: now, Site: SiteStallJudge, Mode: ModeShadow, Outcome: "stalled", Baseline: "stalled", LatencyMs: 300, CostUSD: 0.00002},
		{At: now, Site: SiteStallJudge, Mode: ModeShadow, Outcome: "ok", Baseline: "stalled", LatencyMs: 500},
		{At: now, Site: SiteStallJudge, Mode: ModeShadow, Error: "timeout"},
		{At: now, Site: SiteToolRisk, Mode: ModeOn, Outcome: "ask", Applied: true, LatencyMs: 700},
		{At: now - int64(48*time.Hour/time.Millisecond), Site: SiteToolRisk, Mode: ModeOn, Outcome: "run"},
	}
	for _, r := range recs {
		l.Append(r)
	}

	stats := l.Stats(time.Now().Add(-24 * time.Hour))
	if len(stats) != 2 {
		t.Fatalf("stats = %+v, want two sites", stats)
	}
	stall := stats[0]
	if stall.Site != SiteStallJudge || stall.Calls != 3 || stall.Errors != 1 || stall.Compared != 2 || stall.Agreed != 1 || stall.Shadow != 2 {
		t.Errorf("stall stats = %+v", stall)
	}
	if stall.P50Ms != 300 || stall.P95Ms != 500 {
		t.Errorf("stall latency p50/p95 = %d/%d", stall.P50Ms, stall.P95Ms)
	}
	if risk := stats[1]; risk.Calls != 1 || risk.Applied != 1 {
		t.Errorf("tool-risk stats = %+v (the 48h-old record must fall outside the window)", risk)
	}

	// A fresh ledger over the same directory sees the same history.
	again := OpenLedger(dir, nil)
	if got := again.Recent(10); len(got) != len(recs) || got[0].Site != SiteToolRisk {
		t.Errorf("reloaded recent = %+v", got)
	}
}

func TestLedgerNilSafe(t *testing.T) {
	var l *Ledger
	l.Append(Record{Site: "x"})
	if l.Recent(3) != nil || l.Stats(time.Time{}) != nil {
		t.Error("nil ledger returned data")
	}
}

func TestPercentile(t *testing.T) {
	if p := percentile([]int64{5, 1, 3, 2, 4}, 0.5); p != 3 {
		t.Errorf("p50 = %d", p)
	}
	if p := percentile([]int64{1, 2, 3, 4, 100}, 0.95); p != 100 {
		t.Errorf("p95 = %d", p)
	}
	if percentile(nil, 0.5) != 0 {
		t.Error("empty percentile not 0")
	}
}
