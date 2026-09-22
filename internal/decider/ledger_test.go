package decider

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLedgerStatsAndReload(t *testing.T) {
	dir := t.TempDir()
	l := OpenLedger(dir, nil)
	now := time.Now().UnixMilli()
	recs := []Record{
		{At: now, Authority: testGate, Mode: ModeShadow, Instance: "DM1", Outcome: "stalled", Baseline: "stalled", LatencyMs: 300, CostUSD: 0.00002},
		{At: now, Authority: testGate, Mode: ModeShadow, Instance: "DM1", Outcome: "ok", Baseline: "stalled", LatencyMs: 500},
		{At: now, Authority: testGate, Mode: ModeShadow, Instance: "DM1", Error: "timeout"},
		{At: now, Authority: testGate, Mode: ModeShadow, Role: RoleChallenger, Instance: "DM2", Outcome: "ok", Baseline: "ok", LatencyMs: 90},
		{At: now, Authority: testGate, Mode: ModeShadow, Role: RoleChallenger, Instance: "DM2", Error: "http_503"},
		{At: now, Authority: testExplicit, Mode: ModeOn, Instance: "DM2", Fallback: true, Outcome: "arm2", Applied: true, LatencyMs: 700},
		{At: now - int64(48*time.Hour/time.Millisecond), Authority: testExplicit, Mode: ModeOn, Instance: "DM1", Outcome: "arm1"},
	}
	for _, r := range recs {
		l.Append(r)
	}

	stats := l.Stats(time.Now().Add(-24 * time.Hour))
	if len(stats) != 2 {
		t.Fatalf("stats = %+v, want two authorities", stats)
	}
	explicit, gate := stats[0], stats[1]
	if gate.Authority != testGate || gate.Calls != 3 || gate.Errors != 1 || gate.Compared != 2 || gate.Agreed != 1 || gate.Shadow != 2 {
		t.Errorf("gate stats = %+v", gate)
	}
	if gate.P50Ms != 300 || gate.P95Ms != 500 {
		t.Errorf("gate latency p50/p95 = %d/%d", gate.P50Ms, gate.P95Ms)
	}
	if gate.ChallengerCalls != 2 || gate.ChallengerErrors != 1 || gate.ChallengerCompared != 1 || gate.ChallengerAgreed != 1 || gate.ChallengerP50Ms != 90 || gate.Challenger != "DM2" {
		t.Errorf("gate challenger stats = %+v", gate)
	}
	if explicit.Calls != 1 || explicit.Applied != 1 || explicit.Fallbacks != 1 {
		t.Errorf("explicit stats = %+v (the 48h-old record must fall outside the window)", explicit)
	}

	models := l.ModelStats(time.Now().Add(-24 * time.Hour))
	if len(models) != 2 || models[0].Instance != "DM1" || models[0].Calls != 3 || models[0].Errors != 1 || models[1].Calls != 3 || models[1].Errors != 1 {
		t.Errorf("model stats = %+v", models)
	}

	// A fresh ledger over the same directory sees the same history.
	again := OpenLedger(dir, nil)
	if got := again.Recent(10); len(got) != len(recs) || got[0].Authority != testExplicit {
		t.Errorf("reloaded recent = %+v", got)
	}
}

func TestLedgerReadsSiteLines(t *testing.T) {
	dir := t.TempDir()
	line := `{"at":1790000000000,"site":"stall-judge","mode":"shadow","outcome":"ok","baseline":"ok"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, ledgerFileName), []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	got := OpenLedger(dir, nil).Recent(1)
	if len(got) != 1 || got[0].Authority != "stall-judge" || !got[0].Agrees() {
		t.Errorf("legacy line = %+v", got)
	}
}

func TestLedgerNilSafe(t *testing.T) {
	var l *Ledger
	l.Append(Record{Authority: "x"})
	if l.Recent(3) != nil || l.Stats(time.Time{}) != nil || l.ModelStats(time.Time{}) != nil {
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
