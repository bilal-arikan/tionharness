package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/insight"
)

// TestInsightRecorderOpensNoSessionWithoutAnalysis: the lazy open IS the
// empty-run guard — a recorder that never saw an analysis must leave no session
// and write nothing on finish.
func TestInsightRecorderOpensNoSessionWithoutAnalysis(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()

	rec := newInsightStepRecorder(rt, "IRUN-empty", "AGT1", insightRunTitle(1, 0))
	if err := rec.finish(ctx, insightRunReport{RunID: "IRUN-empty", LensIDs: []string{"tool-errors"}}); err != nil {
		t.Fatalf("finish: %v", err)
	}
	if rec.SessionID() != "" {
		t.Fatalf("no analysis must open no session, got %q", rec.SessionID())
	}
	sessions, err := rt.db.ListSessions(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected no session, got %d", len(sessions))
	}
}

// TestInsightRecorderStepsFollowCompletionOrder: two analyses share ONE session
// and produce one card each (plus the raw reply), persisted in the order they
// completed — so a reload renders the same sequence the live stream showed.
func TestInsightRecorderStepsFollowCompletionOrder(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	a, err := rt.db.CreateAgent(ctx, db.Agent{Name: "A", Provider: "claude-cli"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	rep := insightRunReport{
		RunID:   "IRUN-order",
		LensIDs: []string{"lens-b", "lens-a"},
		AgentID: a.ID,
		Result:  insight.ScanResult{Sessions: 2, Analyzed: 2, Findings: 3},
	}
	rec := newInsightStepRecorder(rt, rep.RunID, rep.AgentID, insightRunTitle(2, 0))

	// lens-b finishes FIRST even though it sorts second: completion order wins.
	rec.captureRaw("lens-b", "SES2", `{"findings":[]}`)
	rec.onAnalysis(insight.AnalysisEvent{LensID: "lens-b", SessionID: "SES2", SessionTitle: "second"})
	rec.onAnalysis(insight.AnalysisEvent{LensID: "lens-a", SessionID: "SES1", SessionTitle: "first", Findings: 3})

	if err := rec.finish(ctx, rep); err != nil {
		t.Fatalf("finish: %v", err)
	}

	msgs, err := rt.db.ListMessages(ctx, rec.SessionID())
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("a scan is ONE turn: got %d messages", len(msgs))
	}
	// Decoded as TurnStep, not view.Step: the narrow view type drops the card ID,
	// which is exactly what the ordering assertion below is about.
	var steps []TurnStep
	if err := json.Unmarshal([]byte(msgs[0].Steps), &steps); err != nil {
		t.Fatalf("decode steps: %v", err)
	}
	// lens-b card, lens-b raw reply, lens-a card.
	if len(steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(steps))
	}
	if steps[0].ID != "lens-b/SES2" || steps[1].ID != "lens-b/SES2/raw" || steps[2].ID != "lens-a/SES1" {
		t.Fatalf("step order broken: %q, %q, %q", steps[0].ID, steps[1].ID, steps[2].ID)
	}
	if steps[0].Tool != insightAnalysisTool {
		t.Fatalf("analysis card tool = %q, want %q", steps[0].Tool, insightAnalysisTool)
	}
	if !strings.Contains(steps[2].Output, "3 bulgu") {
		t.Fatalf("card output must summarise findings, got %q", steps[2].Output)
	}
	if !strings.Contains(string(steps[2].Input), "SES1") {
		t.Fatalf("card input must name the analysed session, got %q", steps[2].Input)
	}
	// The final title carries the run's real counters, not the open-time placeholder.
	sess, err := rt.db.GetSession(ctx, rec.SessionID())
	if err != nil {
		t.Fatal(err)
	}
	if sess.Title != insightRunTitle(2, 3) {
		t.Fatalf("session title = %q, want %q", sess.Title, insightRunTitle(2, 3))
	}
}

// TestInsightRunSessionHeaderCountsTheMessage guards the finish() ordering. The
// message append leaves the on-disk header's MessageCount stale by design (O(1)
// append path), and only a full header write refreshes it — so the retitle must
// come AFTER the message. Read from session.json, not from the in-memory store:
// the in-memory value is right either way, the file is what regressed.
func TestInsightRunSessionHeaderCountsTheMessage(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	a, err := rt.db.CreateAgent(ctx, db.Agent{Name: "A", Provider: "claude-cli"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	rep := insightRunReport{
		RunID:   "IRUN-header",
		LensIDs: []string{"lens-a"},
		AgentID: a.ID,
		Result:  insight.ScanResult{Sessions: 1, Analyzed: 1, Findings: 1},
	}
	rec := newInsightStepRecorder(rt, rep.RunID, rep.AgentID, insightRunTitle(1, 0))
	rec.onAnalysis(insight.AnalysisEvent{LensID: "lens-a", SessionID: "SES1", Findings: 1})
	if err := rec.finish(ctx, rep); err != nil {
		t.Fatalf("finish: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(rt.db.Root(), "sessions", rec.SessionID(), "session.json"))
	if err != nil {
		t.Fatalf("read header: %v", err)
	}
	var header struct {
		MessageCount int `json:"messageCount"`
	}
	if err := json.Unmarshal(raw, &header); err != nil {
		t.Fatalf("decode header: %v", err)
	}
	if header.MessageCount != 1 {
		t.Fatalf("on-disk messageCount = %d, want 1 (retitle must follow the message)", header.MessageCount)
	}
}

// TestInsightRunSessionCarriesScanTagAndFiresTurnHook: the scan session must be
// stamped with insightScanSessionTag AND reach the turn hooks on finish —
// together they are the only path by which a tag automation (the shipped
// insight-apply rule) can ever see a completed scan.
func TestInsightRunSessionCarriesScanTagAndFiresTurnHook(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	a, err := rt.db.CreateAgent(ctx, db.Agent{Name: "A", Provider: "claude-cli"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	fired := make(chan TurnFinished, 1)
	rt.SetTurnHook(func(_ context.Context, tf TurnFinished) { fired <- tf })

	rep := insightRunReport{
		RunID:   "IRUN-tag",
		LensIDs: []string{"lens-a"},
		AgentID: a.ID,
		Result:  insight.ScanResult{Sessions: 1, Analyzed: 1, Findings: 1},
	}
	rec := newInsightStepRecorder(rt, rep.RunID, rep.AgentID, insightRunTitle(1, 0))
	rec.onAnalysis(insight.AnalysisEvent{LensID: "lens-a", SessionID: "SES1", Findings: 1})
	if err := rec.finish(ctx, rep); err != nil {
		t.Fatalf("finish: %v", err)
	}

	sess, err := rt.db.GetSession(ctx, rec.SessionID())
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if !containsTag(sess.Tags, insightScanSessionTag) {
		t.Fatalf("scan session tags = %v, want %q", sess.Tags, insightScanSessionTag)
	}

	select {
	case tf := <-fired:
		if tf.SessionID != rec.SessionID() || tf.AgentID != a.ID {
			t.Fatalf("turn hook got %+v, want session %q agent %q", tf, rec.SessionID(), a.ID)
		}
		if !strings.Contains(tf.Output, rep.RunID) {
			t.Fatalf("turn hook output must carry the run transcript, got %q", tf.Output)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("finish did not fire the turn hook: a tag automation can never see the scan")
	}
}

// TestInsightAnalysisStepsMarkErrors: a failed analysis still gets a card, marked
// as an error so the transcript shows WHICH pair failed rather than going silent.
func TestInsightAnalysisStepsMarkErrors(t *testing.T) {
	steps := insightAnalysisSteps(insight.AnalysisEvent{
		LensID: "lens-a", SessionID: "SES1", Err: context.DeadlineExceeded,
	}, "")
	if len(steps) != 1 {
		t.Fatalf("a failed analysis has no raw reply: got %d steps", len(steps))
	}
	if !steps[0].IsError || !strings.Contains(steps[0].Output, context.DeadlineExceeded.Error()) {
		t.Fatalf("error card = %+v", steps[0])
	}
}
