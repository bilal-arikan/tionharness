package insight

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

// fakeAnalyzer is a deterministic stand-in for the LLM: it counts calls and emits
// whatever `emit` returns, so the whole pipeline is exercised without a model.
// callCount is atomic because the scanner runs Analyze concurrently (bounded pool).
type fakeAnalyzer struct {
	callCount int64
	emit      func(req AnalysisRequest) []Finding
}

func (f *fakeAnalyzer) calls() int { return int(atomic.LoadInt64(&f.callCount)) }

func (f *fakeAnalyzer) Analyze(_ context.Context, req AnalysisRequest) ([]Finding, error) {
	atomic.AddInt64(&f.callCount, 1)
	if f.emit != nil {
		return f.emit(req), nil
	}
	return nil, nil
}

func stepsJSON(kind string) string {
	switch kind {
	case "error":
		return `[{"kind":"error","reason":"provider_error","text":"boom"}]`
	default:
		return `[{"kind":"text","text":"hello"}]`
	}
}

// newScanFixture builds a scanner over a temp store with a tool-errors lens and
// two sessions: SES1 has an error step, SES2 does not.
func newScanFixture(t *testing.T, analyzer Analyzer) (*Scanner, *db.DB, *FindingStore) {
	t.Helper()
	root := t.TempDir()
	database, err := db.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	s1, _ := database.CreateSession(ctx, db.Session{AgentID: "AGT1", Title: "with error"})
	if _, err := database.AddMessage(ctx, db.Message{SessionID: s1.ID, Role: "assistant", Text: "x", Steps: stepsJSON("error")}); err != nil {
		t.Fatal(err)
	}
	s2, _ := database.CreateSession(ctx, db.Session{AgentID: "AGT1", Title: "clean"})
	if _, err := database.AddMessage(ctx, db.Message{SessionID: s2.ID, Role: "assistant", Text: "y", Steps: stepsJSON("text")}); err != nil {
		t.Fatal(err)
	}

	lensDir := t.TempDir()
	if err := EnsureDefaults(lensDir); err != nil {
		t.Fatal(err)
	}
	reg, errs := LoadRegistry(lensDir)
	if len(errs) != 0 {
		t.Fatalf("lens load: %v", errs)
	}
	ledger, err := OpenLedger(root)
	if err != nil {
		t.Fatal(err)
	}
	findings, err := OpenFindingStore(root)
	if err != nil {
		t.Fatal(err)
	}

	clock := int64(1000)
	sc := NewScanner(database, reg, ledger, findings, analyzer, func() int64 { return clock })
	return sc, database, findings
}

func TestScanPrefiltersAndProducesFindings(t *testing.T) {
	fa := &fakeAnalyzer{emit: func(req AnalysisRequest) []Finding {
		return []Finding{{Signature: "boom", Title: "Boom in " + req.SessionID}}
	}}
	sc, _, findings := newScanFixture(t, fa)

	res, err := sc.Scan(context.Background(), ScanScope{LensIDs: []string{"tool-errors"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Sessions != 2 {
		t.Fatalf("expected 2 sessions considered, got %d", res.Sessions)
	}
	if res.Prefiltered != 1 {
		t.Fatalf("clean session should be prefiltered out, got Prefiltered=%d", res.Prefiltered)
	}
	if res.Analyzed != 1 || fa.calls() != 1 {
		t.Fatalf("only the error session should reach the analyzer: Analyzed=%d calls=%d", res.Analyzed, fa.calls())
	}
	if res.Findings != 1 || len(findings.List("", "")) != 1 {
		t.Fatalf("expected 1 finding, got res=%d stored=%d", res.Findings, len(findings.List("", "")))
	}
	if len(res.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", res.Errors)
	}
}

func TestScanIsIncremental(t *testing.T) {
	fa := &fakeAnalyzer{emit: func(req AnalysisRequest) []Finding {
		return []Finding{{Signature: "boom", Title: "Boom"}}
	}}
	sc, database, findings := newScanFixture(t, fa)
	ctx := context.Background()

	if _, err := sc.Scan(ctx, ScanScope{LensIDs: []string{"tool-errors"}}); err != nil {
		t.Fatal(err)
	}
	callsAfterFirst := fa.calls()

	// Re-scan with no changes → every pair skipped by the ledger, analyzer untouched.
	res, err := sc.Scan(ctx, ScanScope{LensIDs: []string{"tool-errors"}})
	if err != nil {
		t.Fatal(err)
	}
	if fa.calls() != callsAfterFirst {
		t.Fatalf("unchanged sessions must not re-hit the analyzer: calls %d -> %d", callsAfterFirst, fa.calls())
	}
	if res.Analyzed != 0 {
		t.Fatalf("re-scan should analyze nothing, got Analyzed=%d", res.Analyzed)
	}
	if res.Skipped == 0 {
		t.Fatal("re-scan should report skipped pairs")
	}

	// Mutate SES1 (new message → fingerprint + UpdatedAt change) → it re-scans;
	// the same signature dedupes into the existing finding (Occurrences=2).
	sessions, _ := database.ListSessions(ctx, "AGT1")
	var ses1 string
	for _, s := range sessions {
		if s.Title == "with error" {
			ses1 = s.ID
		}
	}
	if _, err := database.AddMessage(ctx, db.Message{SessionID: ses1, Role: "assistant", Steps: stepsJSON("error")}); err != nil {
		t.Fatal(err)
	}
	res, err = sc.Scan(ctx, ScanScope{LensIDs: []string{"tool-errors"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Analyzed != 1 || fa.calls() != callsAfterFirst+1 {
		t.Fatalf("changed session should re-scan once: Analyzed=%d calls=%d", res.Analyzed, fa.calls())
	}
	all := findings.List("", "")
	if len(all) != 1 || all[0].Occurrences != 2 {
		t.Fatalf("recurring finding should dedupe to 1 row with occurrences=2, got len=%d occ=%d", len(all), all[0].Occurrences)
	}
}

func TestScanAnalyzerErrorLeavesPairRetryable(t *testing.T) {
	failing := &erroringAnalyzer{}
	sc, _, _ := newScanFixture(t, failing)
	ctx := context.Background()

	res, err := sc.Scan(ctx, ScanScope{LensIDs: []string{"tool-errors"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Errors) != 1 {
		t.Fatalf("analyzer error should be reported, got %v", res.Errors)
	}
	// The failed pair was NOT recorded → a re-scan retries it (analyzer hit again).
	before := failing.calls
	if _, err := sc.Scan(ctx, ScanScope{LensIDs: []string{"tool-errors"}}); err != nil {
		t.Fatal(err)
	}
	if failing.calls != before+1 {
		t.Fatalf("un-recorded pair should be retried: calls %d -> %d", before, failing.calls)
	}
}

// TestScanMaxAnalyzedCaps: the analyzer budget stops queueing calls at the cap,
// and the un-analyzed pairs stay retryable on the next scan.
func TestScanMaxAnalyzedCaps(t *testing.T) {
	root := t.TempDir()
	database, err := db.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	// Five sessions that all trip the tool-errors prefilter.
	for i := 0; i < 5; i++ {
		s, _ := database.CreateSession(ctx, db.Session{AgentID: "AGT1", Title: "err"})
		if _, err := database.AddMessage(ctx, db.Message{SessionID: s.ID, Role: "assistant", Steps: stepsJSON("error")}); err != nil {
			t.Fatal(err)
		}
	}
	lensDir := t.TempDir()
	if err := EnsureDefaults(lensDir); err != nil {
		t.Fatal(err)
	}
	reg, _ := LoadRegistry(lensDir)
	ledger, _ := OpenLedger(root)
	findings, _ := OpenFindingStore(root)
	fa := &fakeAnalyzer{emit: func(req AnalysisRequest) []Finding {
		return []Finding{{Signature: "s-" + req.SessionID, Title: "t"}}
	}}
	sc := NewScanner(database, reg, ledger, findings, fa, func() int64 { return 1000 })

	res, err := sc.Scan(ctx, ScanScope{LensIDs: []string{"tool-errors"}, MaxAnalyzed: 2, Concurrency: 4})
	if err != nil {
		t.Fatal(err)
	}
	if res.Analyzed != 2 || fa.calls() != 2 {
		t.Fatalf("MaxAnalyzed=2 must cap analyzer calls: Analyzed=%d calls=%d", res.Analyzed, fa.calls())
	}
	// The remaining 3 pairs were never recorded → a second scan analyzes them.
	res2, err := sc.Scan(ctx, ScanScope{LensIDs: []string{"tool-errors"}, MaxAnalyzed: 10, Concurrency: 4})
	if err != nil {
		t.Fatal(err)
	}
	if res2.Analyzed != 3 {
		t.Fatalf("remaining pairs should be retried on the next scan: Analyzed=%d", res2.Analyzed)
	}
}

type erroringAnalyzer struct{ calls int }

func (e *erroringAnalyzer) Analyze(_ context.Context, _ AnalysisRequest) ([]Finding, error) {
	e.calls++
	return nil, context.DeadlineExceeded
}
