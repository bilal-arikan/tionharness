package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/insight"
)

// seedRunScopedFindings writes two findings stamped with DIFFERENT scan runs and
// logs the newer run (with its scan session id), so a run-scoped list has
// something to exclude.
func seedRunScopedFindings(t *testing.T) (*db.DB, string, string) {
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
	fresh, err := store.Upsert(insight.Finding{
		LensID: "tool-errors", Channel: insight.ChannelWorkspaceOpt, Signature: "sig-fresh",
		Title: "From this run", LastRunID: "RUN-NEW", LastSeen: 2,
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	old, err := store.Upsert(insight.Finding{
		LensID: "tool-errors", Channel: insight.ChannelWorkspaceOpt, Signature: "sig-old",
		Title: "From an older run", LastRunID: "RUN-OLD", LastSeen: 1,
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := insight.AppendRun(database.Root(), insight.RunRecord{
		ID: "RUN-NEW", SessionID: "SES900", At: 2,
	}); err != nil {
		t.Fatalf("append run: %v", err)
	}
	return database, fresh.ID, old.ID
}

// TestInsightListFindingsRunIDScopesToOneRun: the runId filter is what keeps an
// automation fired by a scan from re-triaging the whole untriaged backlog. It
// must accept the run id AND the run's scan session id (the only identity an
// insight-scan automation has in its prompt variables).
func TestInsightListFindingsRunIDScopesToOneRun(t *testing.T) {
	database, freshID, oldID := seedRunScopedFindings(t)
	tool := NewInsightFindingsTool(database)

	for _, ref := range []string{"RUN-NEW", "SES900"} {
		out, err := tool.Call(context.Background(), json.RawMessage(`{"runId":"`+ref+`"}`))
		if err != nil {
			t.Fatalf("list runId=%s: %v", ref, err)
		}
		if !strings.Contains(out, freshID) {
			t.Fatalf("runId=%s dropped this run's finding:\n%s", ref, out)
		}
		if strings.Contains(out, oldID) {
			t.Fatalf("runId=%s leaked an older run's finding:\n%s", ref, out)
		}
	}

	// Without the filter both findings are listed, so the narrowing above is the
	// filter's doing and not an empty store.
	out, err := tool.Call(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out, freshID) || !strings.Contains(out, oldID) {
		t.Fatalf("unfiltered list should carry both findings:\n%s", out)
	}
}

// TestInsightListFindingsRunIDUnknownIsAnError: an unresolvable run must fail
// loudly. Returning "no findings" would read as "the scan found nothing".
func TestInsightListFindingsRunIDUnknownIsAnError(t *testing.T) {
	database, _, _ := seedRunScopedFindings(t)
	tool := NewInsightFindingsTool(database)

	_, err := tool.Call(context.Background(), json.RawMessage(`{"runId":"RUN-NOPE"}`))
	if err == nil {
		t.Fatal("an unknown runId must be an error, not an empty list")
	}
	if !strings.Contains(err.Error(), "RUN-NOPE") {
		t.Fatalf("error should name the bad ref: %v", err)
	}
}
