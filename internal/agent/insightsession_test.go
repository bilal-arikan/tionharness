package agent

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/insight"
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
}

// TestInsightRunSessionIsReadOnlyTranscript: a run that analysed something opens
// the read-only session kind, links back to the run id, carries the rendered
// report — and raises no unread badge (it is hidden from the default view).
func TestInsightRunSessionIsReadOnlyTranscript(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	a, err := rt.db.CreateAgent(ctx, db.Agent{Name: "A", Provider: "claude-cli"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	rep := insightRunReport{
		RunID:   "IRUN-test",
		LensIDs: []string{"tool-errors"},
		AgentID: a.ID,
		Result:  insight.ScanResult{Sessions: 1, Analyzed: 1, Findings: 1},
	}
	rec := newInsightStepRecorder(rt, rep.RunID, rep.AgentID, insightRunTitle(1, 0))
	rec.onAnalysis(insight.AnalysisEvent{LensID: "tool-errors", SessionID: "SES1", Findings: 1})
	if err := rec.finish(ctx, rep); err != nil {
		t.Fatalf("finish: %v", err)
	}
	sessID := rec.SessionID()
	if sessID == "" {
		t.Fatal("an analysed pair must open the run session")
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
	if sess.SourceID != rep.RunID {
		t.Fatalf("session.SourceID = %q, want run id %q", sess.SourceID, rep.RunID)
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
