package decider

import (
	"context"
	"time"
)

// Challenger comparisons. An authority may name a challenger model: after every
// answer the same request goes to the challenger in the background, and both
// verdicts land in the ledger as one challenger record (Outcome = challenger,
// Baseline = the model that answered). Agreement measures consistency on real
// traffic without changing behaviour. Independently labeled evaluations are
// still required before a new model takes over.
const (
	// maxConcurrentChallenges bounds background challenger calls; a challenge
	// that finds no free slot is skipped, never queued.
	maxConcurrentChallenges = 4
	// challengeDeadline bounds one challenger call, including its retry.
	challengeDeadline = 30 * time.Second
)

func (h *Hub) challenge(ctx context.Context, authority string, mode Mode, challenger string, req Request, primary *Response, threshold float64, o callOptions) {
	verdict := o.outcome
	if verdict == nil {
		verdict = func(r *Response) (string, float64) { return Verdict(r, threshold) }
	}
	baseline, _ := verdict(primary)
	run := o.background
	if run == nil {
		run = func(f func()) { go f() }
	}
	bg := context.WithoutCancel(ctx)
	run(func() {
		// The slot is taken inside the background function, so a runner that
		// declines to run it (a closing runtime) never leaks one.
		select {
		case h.challengeSlots <- struct{}{}:
		default:
			o.trace.forModel(challenger, "challenger").emit(DebugEvent{Stage: "skipped", Error: "challenger_busy"})
			return
		}
		defer func() { <-h.challengeSlots }()
		cctx, cancel := context.WithTimeout(bg, challengeDeadline)
		defer cancel()
		challengeOptions := o
		challengeOptions.role = "challenger"
		resp, err := h.ask(cctx, challenger, req, challengeOptions, false)
		if resp != nil && o.trace != nil {
			resp.DebugID = o.trace.base.TraceID
		}
		rec := NewRecord(authority, mode, resp, err)
		if o.trace != nil {
			rec.DebugID = o.trace.base.TraceID
		}
		rec.Role, rec.Ref, rec.Baseline = RoleChallenger, o.ref, baseline
		if rec.Instance == "" {
			rec.Instance = challenger
		}
		if err == nil {
			rec.Outcome, rec.Strength = verdict(resp)
		} else {
			h.logger.Debug("challenger decision failed", "authority", authority, "model", challenger, "error", err)
		}
		h.Log(rec)
	})
}
