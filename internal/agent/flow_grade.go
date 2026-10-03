package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/decider"
)

// Decision authority "flow-grade" (_Docs/93): after a flow run succeeds, a
// decision model grades the reply against the request on an ordered 1..5
// scale. The grade is the quality signal the system otherwise lacks — most
// turns never get a thumbs up/down — so the observer's evidence, the runs
// list and flow-kind automations ("when graded 2 or below…") can read it.
// It runs off the turn's critical path (afterFlowRun) and never changes the
// reply.

const (
	flowGradeKey         = "grade"
	flowGradeInputRunes  = 4000
	flowGradeOutputRunes = 12000
)

// flowGradeLevels are the scale's levels, lowest first; the grade is the
// 1-based level index.
var flowGradeLevels = []string{
	"useless: ignores or misreads the request, or is empty / broken",
	"weak: touches the request but with notable errors, gaps or confusion",
	"acceptable: addresses the request with minor issues",
	"good: correct, complete and clear",
	"excellent: correct, complete, concise and shows good judgement",
}

// gradeFlowRun grades one successful run and records the grade. It returns the
// updated run and whether a grade was recorded; any failure (authority off, no
// model, low confidence) leaves the run ungraded.
func (r *Runtime) gradeFlowRun(ctx context.Context, run db.FlowRun, caller db.Agent) (db.FlowRun, bool) {
	if run.Status != db.FlowSuccess || strings.TrimSpace(run.Output) == "" {
		return run, false
	}
	if r.deciderMode(authFlowGrade) != decider.ModeOn {
		return run, false
	}
	state := fmt.Sprintf("# Request\n%s\n\n# Reply\n%s",
		truncateRunes(run.Input, flowGradeInputRunes), truncateRunes(run.Output, flowGradeOutputRunes))
	question := decider.Score("How well does the reply satisfy the request? Judge correctness, completeness and fit to what was asked; "+
		"ignore style unless it hurts usefulness. A reply that asks a necessary clarifying question can still be good.", flowGradeLevels...)
	threshold := r.deciderThreshold(authFlowGrade)
	outcome := func(resp *decider.Response) (string, float64) {
		grade, strength, ok := flowGradeOf(resp, threshold)
		if !ok {
			return "unsure", strength
		}
		return fmt.Sprintf("g%d", grade), strength
	}
	resp, err := r.decide(ctx, authFlowGrade, caller, decider.Request{
		State:     state,
		Questions: map[string]decider.Question{flowGradeKey: question},
	}, decider.WithRef(run.ID), decider.WithOutcome(outcome))
	rec := decider.NewRecord(authFlowGrade, decider.ModeOn, resp, err)
	rec.Ref = run.ID
	if err != nil {
		if !decisionOff(err) {
			r.logDecision(rec)
		}
		return run, false
	}
	grade, strength, ok := flowGradeOf(resp, threshold)
	rec.Outcome, rec.Strength = outcome(resp)
	if !ok {
		r.logDecision(rec)
		return run, false
	}
	rec.Applied = true
	r.logDecision(rec)
	graded, gerr := r.db.SetFlowRunGrade(ctx, run.ID, grade, strength)
	if gerr != nil {
		r.logger.Warn("flow grade: record failed", "run", run.ID, "error", gerr)
		return run, false
	}
	return graded, true
}

// flowGradeOf reads the score answer as a 1..5 grade with its strength; ok is
// false for a missing answer or one below the authority's threshold.
func flowGradeOf(resp *decider.Response, threshold float64) (int, float64, bool) {
	if resp == nil {
		return 0, 0, false
	}
	a, found := resp.Answers[flowGradeKey]
	if !found || a.Type != decider.QuestionScore {
		return 0, 0, false
	}
	grade := a.Level() + 1
	if grade < 1 {
		grade = 1
	}
	if grade > len(flowGradeLevels) {
		grade = len(flowGradeLevels)
	}
	s := a.Strength()
	return grade, s, s >= threshold
}

// FlowRunFinished is the signal a finished (and, when enabled, graded) flow
// run delivers to the flow-run hooks (the automation engine's flow rules).
type FlowRunFinished struct {
	Run db.FlowRun
}

// afterFlowRun is the detached tail of a flow run: grade it, tell the
// flow-run hooks, and let the observer look. Nothing here delays the reply.
func (r *Runtime) afterFlowRun(ctx context.Context, f db.Flow, run db.FlowRun, agent db.Agent) {
	bg := context.WithoutCancel(ctx)
	work := func() {
		if graded, ok := r.gradeFlowRun(bg, run, agent); ok {
			run = graded
			r.emitFlowRunEvent(run)
		}
		r.FireFlowRunFinished(FlowRunFinished{Run: run})
		if run.Status == db.FlowSuccess {
			r.maybeObserveFlow(bg, f.ID)
		}
	}
	if !r.startBackgroundTurn(work) {
		r.logger.Debug("flow: post-run work skipped, workspace closing", "run", run.ID)
	}
}
