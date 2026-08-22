package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/insight"
)

// insightAnalysisTool is the card name a live analysis step renders under. It is
// not a callable tool — the scan drives the analyzer itself — but the step trace
// (and the UI that reads it) is uniform, so an analysis reads like any other
// tool call: input = the (lens, session) pair, output = the finding rollup.
const insightAnalysisTool = "insight_analyze"

// insightRawCap bounds how much of the model's raw reply is kept per analysis.
// The reply is JSON findings plus occasional prose; a runaway one must not blow
// up the persisted transcript.
const insightRawCap = 4000

// insightStepRecorder turns a scan into a normal agent turn: every completed
// (lens, session) analysis becomes a live step card AND lands in the run
// session's persisted step trace, in the SAME order (completion order), so a
// reload does not reshuffle the cards.
//
// The scan's session is opened LAZILY, on the first analysis: a scan that
// analysed nothing (all pairs ledger-skipped or prefiltered) writes no
// transcript at all, which is what keeps the hourly cron from piling up empty
// sessions.
//
// The scanner calls onAnalysis from its concurrent phase-2 workers, so every
// method here is mutex-guarded.
type insightStepRecorder struct {
	rt      *Runtime
	runID   string
	agentID string
	title   string // session title used at open time

	mu        sync.Mutex
	raw       map[string]string // pair key → model's raw reply, awaiting its event
	steps     []TurnStep
	sessionID string
	openErr   error
}

func newInsightStepRecorder(rt *Runtime, runID, agentID, title string) *insightStepRecorder {
	return &insightStepRecorder{rt: rt, runID: runID, agentID: agentID, title: title, raw: map[string]string{}}
}

// insightPairKey is the live-card identity of one (lens, session) analysis.
func insightPairKey(lensID, sessionID string) string { return lensID + "/" + sessionID }

// captureRaw stashes the model's verbatim reply for a pair. The analyzer calls
// it (the Analyzer interface only returns parsed findings, and widening that
// interface for observability would push LLM-response plumbing into every fake);
// the matching AnalysisEvent then picks it up and renders it as a thinking card.
func (rec *insightStepRecorder) captureRaw(lensID, sessionID, text string) {
	if rec == nil {
		return
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	rec.raw[insightPairKey(lensID, sessionID)] = truncateRunes(text, insightRawCap)
}

// SessionID reports the lazily opened session id ("" when the scan never
// analysed anything, or when opening failed).
func (rec *insightStepRecorder) SessionID() string {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return rec.sessionID
}

// onAnalysis is the insight.AnalysisSink: one completed analysis → one tool card
// (+ the raw reply as a thinking card), broadcast live and appended to the trace.
func (rec *insightStepRecorder) onAnalysis(ev insight.AnalysisEvent) {
	rec.mu.Lock()
	defer rec.mu.Unlock()

	key := insightPairKey(ev.LensID, ev.SessionID)
	raw := rec.raw[key]
	delete(rec.raw, key)

	steps := insightAnalysisSteps(ev, raw)
	rec.steps = append(rec.steps, steps...)

	sid := rec.openSessionLocked()
	if sid == "" {
		return // openErr already logged; the steps stay for a later retry/finish
	}
	// Emitted under the lock on purpose: the live order must equal the persisted
	// order, otherwise cards move around after a reload.
	for _, st := range steps {
		rec.rt.emitSessionStep(sid, st, "")
	}
}

// openSessionLocked creates the run's transcript session once. A failed open is
// remembered so a long scan does not retry (and re-log) it on every analysis.
func (rec *insightStepRecorder) openSessionLocked() string {
	if rec.sessionID != "" || rec.openErr != nil {
		return rec.sessionID
	}
	sess, err := rec.rt.openInsightSession(context.Background(), rec.runID, rec.agentID, rec.title)
	if err != nil {
		rec.openErr = err
		rec.rt.logger.Warn("insight run session open failed", "error", err, "run", rec.runID)
		return ""
	}
	rec.sessionID = sess
	return sess
}

// insightAnalysisSteps renders one analysis as the cards a turn shows: the
// tool-shaped analysis card, then the model's verbatim reply as thinking.
func insightAnalysisSteps(ev insight.AnalysisEvent, raw string) []TurnStep {
	key := insightPairKey(ev.LensID, ev.SessionID)
	input, err := json.Marshal(map[string]string{
		"lens":         ev.LensID,
		"session":      ev.SessionID,
		"sessionTitle": ev.SessionTitle,
	})
	if err != nil {
		// A map[string]string cannot fail to marshal; if it ever does, the card is
		// still worth showing without its input rather than dropping the analysis.
		input = nil
	}
	card := TurnStep{
		Kind:  StepTool,
		ID:    key,
		Tool:  insightAnalysisTool,
		Input: input,
	}
	if ev.Err != nil {
		card.IsError = true
		card.Output = fmt.Sprintf("analiz başarısız (%.1fs): %v", ev.Duration.Seconds(), ev.Err)
	} else {
		card.Output = fmt.Sprintf("%d bulgu • %.1fs", ev.Findings, ev.Duration.Seconds())
	}
	steps := []TurnStep{card}
	if raw != "" {
		steps = append(steps, TurnStep{Kind: StepThinking, ID: key + "/raw", Text: raw})
	}
	return steps
}

// finish writes the accumulated cards as ONE assistant message (the scan's turn)
// and retitles the session with the final counters. A scan that opened no
// session is a no-op. Errors are returned so the caller can log them and still
// write the run-log row.
func (rec *insightStepRecorder) finish(ctx context.Context, rep insightRunReport) error {
	rec.mu.Lock()
	sid, steps := rec.sessionID, rec.steps
	rec.mu.Unlock()
	if sid == "" {
		return rec.openErr
	}
	// Message FIRST, retitle SECOND. AddMessage appends the transcript line but
	// deliberately leaves the header's MessageCount/UpdatedAt stale on disk (the
	// O(1) append path); the next full header write picks them up. Retitling is
	// exactly such a write, so doing it last leaves session.json consistent —
	// title-then-message would freeze "messageCount": 0 on disk until the session
	// is next mutated, which anything reading the file directly (counter
	// automations, external tooling) would believe.
	if err := rec.rt.recordAssistantMessage(ctx, sid, rep.AgentID,
		insightRunTranscript(rep), steps, nil, rep.Duration.Milliseconds()); err != nil {
		return err
	}
	title := insightRunTitle(len(rep.LensIDs), rep.Result.Findings)
	return rec.rt.db.SetSessionTitle(ctx, sid, title)
}

// openInsightSession creates the read-only transcript session for a scan run.
// SourceID is the run id, so the run-log row and the session resolve each other.
func (r *Runtime) openInsightSession(ctx context.Context, runID, agentID, title string) (string, error) {
	if r == nil || r.db == nil {
		return "", fmt.Errorf("insight: runtime not ready")
	}
	sess, err := r.db.CreateSession(ctx, db.Session{
		AgentID:  agentID,
		Kind:     db.SessionKindInsight,
		SourceID: runID,
		Title:    title,
	})
	if err != nil {
		return "", err
	}
	return sess.ID, nil
}
