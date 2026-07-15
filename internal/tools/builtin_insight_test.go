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
		Title: "Bash disabled but advertised", Severity: "high", ProposedFix: "filter tool list", LastSeen: 1,
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
