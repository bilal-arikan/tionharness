package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/decider"
)

// Decider site "phase-gate": a Rota phase gate of kind "judge". The gate's value
// is the phase's exit condition in plain words ("the reviewer approved the
// change and no blocking issue is open"); the decision model judges it against
// the root session's recent transcript. Unlike the "verdict" gate it needs no
// exact marker line, so a validator that words its approval differently — or
// wraps it in a code fence — still opens the gate, and a template echo of
// "VERDICT: PASS | FAIL" does not.
//
// The gate fails closed: when the decider cannot answer, the phase stays
// blocked with the reason, exactly like an unmet condition.

const (
	phaseGateKey = "holds"
	// phaseGateTailMessages is how much of the root transcript is judged.
	phaseGateTailMessages = 40
	// phaseGateMessageRunes caps one message inside the judged transcript.
	phaseGateMessageRunes = 2000
)

// judgePhaseGate evaluates a "judge" gate. It returns pass, or the reason the
// phase stays blocked.
func (r *Runtime) judgePhaseGate(ctx context.Context, t db.Trajectory, phase db.TrajectoryNode, condition string) (bool, string) {
	if condition == "" {
		return false, "judge gate has no condition to judge"
	}
	msgs, _, _ := r.db.ListMessagesTail(ctx, t.RootSessionID, phaseGateTailMessages)
	transcript := renderGateTranscript(msgs)
	if transcript == "" {
		return false, "judge gate: the root transcript is empty"
	}
	label := phase.Label
	if label == "" {
		label = phase.ID
	}
	resp, err := r.decide(ctx, decider.SitePhaseGate, r.gateCaller(ctx, t), decider.Request{
		State: transcript,
		Questions: map[string]decider.Question{phaseGateKey: decider.Noul(
			fmt.Sprintf("Judging by this transcript, has the exit condition of the %q phase been met? Condition: %s", label, condition),
			condition,
			"The condition has not been met yet, or the transcript does not show that it has.")},
	})
	rec := decider.NewRecord(decider.SitePhaseGate, decider.ModeOn, resp, err)
	rec.Ref = t.ID
	if err != nil {
		if !decisionOff(err) {
			r.logDecision(rec)
		}
		return false, "judge gate unavailable: " + err.Error()
	}
	a := resp.Answers[phaseGateKey]
	threshold := r.deciderThreshold(decider.SitePhaseGate)
	pass := a.Yes(threshold)
	rec.Outcome, rec.Strength, rec.Applied = "blocked", a.Probability, true
	if pass {
		rec.Outcome = "pass"
	}
	r.logDecision(rec)
	if pass {
		return true, ""
	}
	return false, fmt.Sprintf("judge: condition not met (p=%.2f, needs %.2f)", a.Probability, threshold)
}

// gateCaller is the agent a gate judgement is billed to: the root session's
// agent (zero when it cannot be resolved; the ledger still records the cost).
func (r *Runtime) gateCaller(ctx context.Context, t db.Trajectory) db.Agent {
	sess, err := r.db.GetSession(ctx, t.RootSessionID)
	if err != nil || sess.AgentID == "" {
		return db.Agent{}
	}
	a, err := r.db.GetAgent(ctx, sess.AgentID)
	if err != nil {
		return db.Agent{}
	}
	return a
}

// renderGateTranscript flattens messages into "role: text" blocks, oldest
// first, each capped so one huge tool dump cannot crowd out the rest.
func renderGateTranscript(msgs []db.Message) string {
	var b strings.Builder
	for _, m := range msgs {
		text := strings.TrimSpace(m.Text)
		if text == "" {
			continue
		}
		fmt.Fprintf(&b, "%s: %s\n\n", m.Role, truncateRunes(text, phaseGateMessageRunes))
	}
	return strings.TrimSpace(b.String())
}
