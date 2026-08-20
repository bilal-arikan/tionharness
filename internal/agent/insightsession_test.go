package agent

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/insight"
)

// TestInsightRunSkipsEmptyScan: a run that did no work (no analysis, no finding,
// no error) leaves the cheap run-log row ONLY — no empty transcript, so the
// hourly cron cannot pile up identical blank sessions.
func TestInsightRunSkipsEmptyScan(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	if _, err := rt.db.CreateAgent(ctx, db.Agent{Name: "A", Provider: "claude-cli"}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	// Empty store: nothing to analyze.
	if _, err := rt.RunInsightScan(ctx, insight.ScanScope{LensIDs: []string{"tool-errors"}}, ""); err != nil {
		t.Fatalf("scan: %v", err)
	}

	runs, err := insight.ReadRuns(rt.db.Root(), 10)
	if err != nil {
		t.Fatalf("read runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected exactly one run record, got %d", len(runs))
	}
	if runs[0].SessionID != "" {
		t.Fatalf("empty scan must not open a session, got %q", runs[0].SessionID)
	}
	sessions, err := rt.db.ListSessions(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range sessions {
		if s.Kind == db.SessionKindInsight {
			t.Fatalf("empty scan opened an insight session: %s", s.ID)
		}
	}

	// The predicate itself: any real work re-enables the transcript.
	base := insightRunReport{RunID: "IRUN-x", LensIDs: []string{"tool-errors"}}
	if insightRunSessionWorthy(base) {
		t.Fatal("a no-work run must not be session-worthy")
	}
	if insightRunSessionWorthy(insightRunReport{RunID: "IRUN-x"}) {
		t.Fatal("a run with no resolved lens must not be session-worthy")
	}
	base.Result.Analyzed = 1
	if !insightRunSessionWorthy(base) {
		t.Fatal("an analyzed pair must make the run session-worthy")
	}
}

// TestRecordInsightSessionWritesReadOnlyTranscript: a worthy run's session is the
// read-only kind, links back to the run id, carries the rendered report — and
// raises no unread badge (it is hidden from the default sessions view).
func TestRecordInsightSessionWritesReadOnlyTranscript(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	a, err := rt.db.CreateAgent(ctx, db.Agent{Name: "A", Provider: "claude-cli"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	rec := insight.RunRecord{ID: "IRUN-test"}
	sessID, err := rt.recordInsightSession(ctx, insightRunReport{
		RunID:   rec.ID,
		LensIDs: []string{"tool-errors"},
		AgentID: a.ID,
		Result:  insight.ScanResult{Sessions: 1, Analyzed: 1, Findings: 1},
	})
	if err != nil {
		t.Fatalf("record session: %v", err)
	}

	sess, err := rt.db.GetSession(ctx, sessID)
	if err != nil {
		t.Fatalf("run session %s not found: %v", sessID, err)
	}
	if sess.Unread {
		t.Fatal("machine-written transcript must not raise an unread badge")
	}
	if sess.Kind != db.SessionKindInsight {
		t.Fatalf("run session kind = %q, want %q", sess.Kind, db.SessionKindInsight)
	}
	if sess.SourceID != rec.ID {
		t.Fatalf("session.SourceID = %q, want run id %q", sess.SourceID, rec.ID)
	}
	// The transcript is written, not empty: the report turn must be there.
	msgs, err := rt.db.ListMessages(ctx, sess.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected one report message, got %d", len(msgs))
	}
	if msgs[0].Text == "" {
		t.Fatal("report message must carry the rendered transcript")
	}
}
