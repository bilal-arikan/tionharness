package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/insight"
)

// seedFindings opens a temp DB and writes two findings (one new, one dismissed)
// into its insight store, returning the DB and the "new" finding's id.
func seedFindings(t *testing.T) (*db.DB, string) {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	store, err := insight.OpenFindingStore(database.Root())
	if err != nil {
		t.Fatalf("open findings: %v", err)
	}
	f, err := store.Upsert(insight.Finding{
		LensID: "tool-errors", Channel: insight.ChannelAppFix, Signature: "sigA",
		Title: "Bash disabled but advertised", Severity: "high", RootCause: "registry advertises a disabled tool",
		ProposedFix: "filter tool list", FilePointer: "internal/tools/registry.go", LastSeen: 1,
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	other, _ := store.Upsert(insight.Finding{
		LensID: "context-hygiene", Channel: insight.ChannelWorkspaceOpt, Signature: "sigB",
		Title: "Old thing", LastSeen: 2,
	})
	if _, err := store.SetStatus(other.ID, insight.StatusDismissed, 3); err != nil {
		t.Fatalf("set status: %v", err)
	}
	return database, f.ID
}

// TestInsightListFindingsShowsIDAndFiltersStatus proves the list output leads
// with the finding id (so insight_apply_finding is callable) and that the status
// filter narrows results — the agent workflow depends on both.
func TestInsightListFindingsShowsIDAndFiltersStatus(t *testing.T) {
	database, newID := seedFindings(t)
	tool := NewInsightFindingsTool(database)

	out, err := tool.Call(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out, newID) {
		t.Fatalf("list output must contain the finding id %q:\n%s", newID, out)
	}
	if !strings.Contains(out, "2 finding") {
		t.Fatalf("expected 2 findings listed:\n%s", out)
	}

	// status:new must exclude the dismissed one.
	outNew, err := tool.Call(context.Background(), json.RawMessage(`{"status":"new"}`))
	if err != nil {
		t.Fatalf("list new: %v", err)
	}
	if !strings.Contains(outNew, "1 finding") || !strings.Contains(outNew, newID) {
		t.Fatalf("status:new should return exactly the new finding:\n%s", outNew)
	}
	if strings.Contains(outNew, "Old thing") {
		t.Fatalf("status:new must not include the dismissed finding:\n%s", outNew)
	}
}

// stubScanner returns a canned ScanResult so the tool's summary formatting can
// be tested without a runtime.
type stubScanner struct{ res insight.ScanResult }

func (s stubScanner) RunInsightScan(context.Context, insight.ScanScope, string) (insight.ScanResult, error) {
	return s.res, nil
}

// TestInsightScanDedupsRepeatedErrors proves 53 identical provider errors collapse
// into a single line carrying the full message once plus its count.
func TestInsightScanDedupsRepeatedErrors(t *testing.T) {
	const msg = "codex: model deepseek-v4-flash unavailable"
	errs := make([]string, 0, 54)
	for i := 0; i < 53; i++ {
		errs = append(errs, msg)
	}
	errs = append(errs, "other: session read failed")
	tool := NewInsightScanTool(stubScanner{res: insight.ScanResult{Sessions: 53, Errors: errs}})

	out, err := tool.Call(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if !strings.Contains(out, "54 error(s)") {
		t.Fatalf("total error count must stay visible:\n%s", out)
	}
	if !strings.Contains(out, msg+" (×53)") {
		t.Fatalf("repeated error must appear once with a count:\n%s", out)
	}
	if strings.Count(out, msg) != 1 {
		t.Fatalf("repeated error must not be repeated:\n%s", out)
	}
	if !strings.Contains(out, "other: session read failed") {
		t.Fatalf("distinct error must survive:\n%s", out)
	}
}

// TestSummarizeScanErrorsTrimsSignatures checks the "+N more" tail.
func TestSummarizeScanErrorsTrimsSignatures(t *testing.T) {
	got := summarizeScanErrors([]string{"err-1", "err-2", "err-3", "err-4", "err-5"})
	if !strings.Contains(got, "+2 more distinct error(s)") {
		t.Fatalf("expected a trimmed tail, got %q", got)
	}
	if strings.Contains(got, "err-4") || strings.Contains(got, "err-5") {
		t.Fatalf("signatures past the cap must be trimmed, got %q", got)
	}
}

// TestInsightListFindingsVerboseModes proves the default is a one-line summary
// (ids still present, cause/fix omitted) and verbose:true restores the detail.
func TestInsightListFindingsVerboseModes(t *testing.T) {
	database, newID := seedFindings(t)
	tool := NewInsightFindingsTool(database)

	compact, err := tool.Call(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(compact, newID) {
		t.Fatalf("ids must stay visible in summary mode:\n%s", compact)
	}
	for _, banned := range []string{"cause:", "fix:", "file:", "registry advertises a disabled tool"} {
		if strings.Contains(compact, banned) {
			t.Fatalf("summary mode must not print %q:\n%s", banned, compact)
		}
	}
	if !strings.Contains(compact, "verbose:true") {
		t.Fatalf("summary mode must point at verbose:true:\n%s", compact)
	}

	verbose, err := tool.Call(context.Background(), json.RawMessage(`{"verbose":true}`))
	if err != nil {
		t.Fatalf("list verbose: %v", err)
	}
	for _, want := range []string{newID, "cause: registry advertises a disabled tool", "fix: filter tool list", "file: internal/tools/registry.go"} {
		if !strings.Contains(verbose, want) {
			t.Fatalf("verbose output must contain %q:\n%s", want, verbose)
		}
	}
}

// TestInsightApplyFindingSetsStatus verifies the triage tool records a decision.
func TestInsightApplyFindingSetsStatus(t *testing.T) {
	database, newID := seedFindings(t)
	tool := NewInsightApplyFindingTool(database)

	out, err := tool.Call(context.Background(), json.RawMessage(`{"id":"`+newID+`","status":"accepted"}`))
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !strings.Contains(out, "accepted") {
		t.Fatalf("unexpected apply result: %s", out)
	}
	// An unknown id must error, not silently pass.
	if _, err := tool.Call(context.Background(), json.RawMessage(`{"id":"nope","status":"accepted"}`)); err == nil {
		t.Fatal("applying to an unknown id should error")
	}
	// An invalid status must error.
	if _, err := tool.Call(context.Background(), json.RawMessage(`{"id":"`+newID+`","status":"bogus"}`)); err == nil {
		t.Fatal("an invalid status should error")
	}
}
