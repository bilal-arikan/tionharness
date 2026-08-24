package insight

import (
	"context"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// TestScanExcludesMachineTranscripts: a scan must not feed on its own output —
// the read-only insight run session is out of scope by default, and it does not
// inflate the "sessions considered" counter either.
func TestScanExcludesMachineTranscripts(t *testing.T) {
	var analyzedIDs []string
	fa := &fakeAnalyzer{emit: func(req AnalysisRequest) []Finding {
		analyzedIDs = append(analyzedIDs, req.SessionID)
		return []Finding{{Signature: "boom", Title: "Boom in " + req.SessionID}}
	}}
	sc, database, _ := newScanFixture(t, fa)
	ctx := context.Background()

	// A previous run's transcript: same error-looking content, but machine-written.
	ins, err := database.CreateSession(ctx, db.Session{AgentID: "AGT1", Kind: db.SessionKindInsight, SourceID: "IRUN-1", Title: "run"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.AddMessage(ctx, db.Message{SessionID: ins.ID, Role: "assistant", Text: "report", Steps: stepsJSON("error")}); err != nil {
		t.Fatal(err)
	}

	res, err := sc.Scan(ctx, ScanScope{LensIDs: []string{"tool-errors"}})
	if err != nil {
		t.Fatal(err)
	}
	// The fixture has exactly two user-driven sessions; the insight one must not
	// be counted as considered, skipped or analyzed.
	if res.Sessions != 2 {
		t.Fatalf("insight session must be out of scope entirely: sessions=%d, want 2", res.Sessions)
	}
	if res.Analyzed != 1 {
		t.Fatalf("only the error session should be analyzed: analyzed=%d", res.Analyzed)
	}
	for _, id := range analyzedIDs {
		if id == ins.ID {
			t.Fatalf("scan analyzed its own run session %s", ins.ID)
		}
	}

	// An explicit empty exclusion list opts back in (the mechanism is general).
	sc2, _, _ := newScanFixture(t, fa)
	res2, err := sc2.Scan(ctx, ScanScope{LensIDs: []string{"tool-errors"}, ExcludeKinds: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	if res2.Sessions != 2 {
		t.Fatalf("empty exclusion should consider every session: sessions=%d", res2.Sessions)
	}
}
