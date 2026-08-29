package insight

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// lensFile writes a minimal lens definition that matches any session carrying an
// error signal, so a fixture can put several lenses on the same session.
func lensFile(t *testing.T, dir, id string, channel Channel) {
	t.Helper()
	raw := "---\n" +
		"id: " + id + "\n" +
		"name: \"" + id + "\"\n" +
		"description: \"test lens\"\n" +
		"channel: " + string(channel) + "\n" +
		"enabled: true\n" +
		"prefilter:\n" +
		"  requiresAny: [error]\n" +
		"---\n\n" +
		"Prompt body for " + id + ".\n"
	if err := os.WriteFile(filepath.Join(dir, id+".md"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newMultiLensFixture builds a scanner over one erroring session and two lenses
// that both match it — the shape the per-session grouping exists for.
func newMultiLensFixture(t *testing.T, analyzer Analyzer) (*Scanner, *FindingStore) {
	t.Helper()
	root := t.TempDir()
	database, err := db.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	agent, err := database.CreateAgent(ctx, db.Agent{Name: "fixture agent"})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := database.CreateSession(ctx, db.Session{AgentID: agent.ID, Title: "with error"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.AddMessage(ctx, db.Message{
		SessionID: sess.ID, Role: "assistant", Text: "x", Steps: stepsJSON("error"),
	}); err != nil {
		t.Fatal(err)
	}

	lensDir := t.TempDir()
	lensFile(t, lensDir, "lens-a", ChannelAppFix)
	lensFile(t, lensDir, "lens-b", ChannelWorkspaceOpt)
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
	return sc, findings
}

// TestScanGroupsLensesIntoOneCall is the cost fix: two lenses due for the same
// session must cost ONE analyzer call carrying both, not one call each.
func TestScanGroupsLensesIntoOneCall(t *testing.T) {
	var mu sync.Mutex
	var seen []AnalysisRequest
	fa := &fakeAnalyzer{emit: func(req AnalysisRequest) []Finding {
		mu.Lock()
		seen = append(seen, req)
		mu.Unlock()
		return []Finding{
			{LensID: "lens-a", Signature: "sig-a", Title: "A"},
			{LensID: "lens-b", Signature: "sig-b", Title: "B"},
		}
	}}
	sc, findings := newMultiLensFixture(t, fa)

	res, err := sc.Scan(context.Background(), ScanScope{RunID: "RUN1"})
	if err != nil {
		t.Fatal(err)
	}
	if fa.calls() != 1 {
		t.Fatalf("two due lenses on one session must cost ONE call, got %d", fa.calls())
	}
	if len(seen) != 1 || len(seen[0].LensList()) != 2 {
		t.Fatalf("the call must carry both lenses, got %+v", seen)
	}
	// Analyzed still counts (lens,session) pairs, so budgets/ledger stay comparable.
	if res.Analyzed != 2 {
		t.Fatalf("Analyzed = %d, want 2 pairs", res.Analyzed)
	}
	if len(res.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", res.Errors)
	}

	// Each finding lands on the lens it was tagged with, with that lens's channel.
	stored := findings.List("", "")
	if len(stored) != 2 {
		t.Fatalf("want 2 stored findings, got %d: %+v", len(stored), stored)
	}
	byLens := map[string]Finding{}
	for _, f := range stored {
		byLens[f.LensID] = f
	}
	if got := byLens["lens-a"]; got.Channel != ChannelAppFix || got.Title != "A" {
		t.Fatalf("lens-a finding mis-routed: %+v", got)
	}
	if got := byLens["lens-b"]; got.Channel != ChannelWorkspaceOpt || got.Title != "B" {
		t.Fatalf("lens-b finding mis-routed: %+v", got)
	}
	for _, f := range stored {
		if f.LastRunID != "RUN1" {
			t.Fatalf("finding %s not stamped with the run: %q", f.ID, f.LastRunID)
		}
	}

	// EVERY lens of the group is recorded, so a re-scan skips both halves instead
	// of re-analyzing the ones the single reply also answered for.
	res2, err := sc.Scan(context.Background(), ScanScope{RunID: "RUN2"})
	if err != nil {
		t.Fatal(err)
	}
	if res2.Analyzed != 0 || fa.calls() != 1 {
		t.Fatalf("re-scan re-analyzed: analyzed=%d calls=%d", res2.Analyzed, fa.calls())
	}
	if res2.Skipped != 2 {
		t.Fatalf("re-scan should skip both lens pairs, got Skipped=%d", res2.Skipped)
	}
}

// TestScanReportsUnknownLensTag: a finding tagged with a lens that is not in the
// group is attributed to the first lens AND reported — never dropped silently.
func TestScanReportsUnknownLensTag(t *testing.T) {
	fa := &fakeAnalyzer{emit: func(AnalysisRequest) []Finding {
		return []Finding{{LensID: "not-a-lens", Signature: "sig-x", Title: "X"}}
	}}
	sc, findings := newMultiLensFixture(t, fa)

	res, err := sc.Scan(context.Background(), ScanScope{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Findings != 1 {
		t.Fatalf("the mis-tagged finding must still be stored, got %d", res.Findings)
	}
	stored := findings.List("", "")
	if len(stored) != 1 || stored[0].LensID != "lens-a" {
		t.Fatalf("want fallback to the first lens, got %+v", stored)
	}
	joined := strings.Join(res.Errors, " | ")
	if !strings.Contains(joined, "unknown lensId") {
		t.Fatalf("the mis-tag must be reported, errors = %v", res.Errors)
	}
}

// TestScanDoesNotRecordUnansweredGroupedLens prevents grouped partial replies
// from turning omitted lenses into false-clean ledger entries.
func TestScanDoesNotRecordUnansweredGroupedLens(t *testing.T) {
	var calls int
	fa := &fakeAnalyzer{emit: func(req AnalysisRequest) []Finding {
		calls++
		if calls == 1 {
			return []Finding{{LensID: "lens-a", AnswerOnly: true}}
		}
		if got := req.LensList(); len(got) != 1 || got[0].ID != "lens-b" {
			t.Fatalf("retry lenses = %+v, want only unanswered lens-b", got)
		}
		return nil
	}}
	sc, _ := newMultiLensFixture(t, fa)

	first, err := sc.Scan(context.Background(), ScanScope{})
	if err != nil {
		t.Fatal(err)
	}
	if first.Analyzed != 1 {
		t.Fatalf("Analyzed = %d, want only answered lens-a", first.Analyzed)
	}
	if joined := strings.Join(first.Errors, " | "); !strings.Contains(joined, "lens lens-b not answered in grouped call") {
		t.Fatalf("missing unanswered-lens error: %v", first.Errors)
	}
	if _, err := sc.Scan(context.Background(), ScanScope{}); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, unanswered lens was not retried", calls)
	}
}

func TestScanAnalyzerErrorLeavesGroupedLedgerEmpty(t *testing.T) {
	fa := &erroringAnalyzer{}
	sc, _ := newMultiLensFixture(t, fa)
	for i := 0; i < 2; i++ {
		res, err := sc.Scan(context.Background(), ScanScope{})
		if err != nil {
			t.Fatal(err)
		}
		if res.Analyzed != 0 {
			t.Fatalf("attempt %d recorded analyzer failure: Analyzed=%d", i+1, res.Analyzed)
		}
	}
	if fa.calls != 2 {
		t.Fatalf("calls = %d, want retry proving no ledger record", fa.calls)
	}
}

// TestAttributeFindingsSingleLensIgnoresTag: a one-lens call is unambiguous, so
// whatever the model wrote in lensId is overridden rather than treated as a
// mis-tag.
func TestAttributeFindingsSingleLensIgnoresTag(t *testing.T) {
	lenses := []Lens{{ID: "only"}}
	byLens, answered, unattributed := attributeFindings(lenses, []Finding{{LensID: "garbage", Title: "t"}})
	if unattributed != 0 {
		t.Fatalf("single-lens call reported %d unattributed", unattributed)
	}
	if len(byLens["only"]) != 1 {
		t.Fatalf("finding not attributed to the only lens: %+v", byLens)
	}
	if !answered["only"] {
		t.Fatal("single-lens response must remain answered for compatibility")
	}
}

// TestAnalysisRequestLensListFallsBackToSingleLens keeps the pre-grouping shape
// working: a request built with only Lens still reports one lens.
func TestAnalysisRequestLensListFallsBackToSingleLens(t *testing.T) {
	req := AnalysisRequest{Lens: Lens{ID: "solo"}}
	got := req.LensList()
	if len(got) != 1 || got[0].ID != "solo" {
		t.Fatalf("LensList() = %+v, want the single Lens", got)
	}
}
