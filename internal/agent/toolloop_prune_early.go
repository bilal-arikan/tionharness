package agent

import (
	"github.com/bilal-arikan/tionharness/internal/conversation"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// earlyPruneRatio is the share of the model's context window the in-flight
// request may occupy before the loop prunes old tool-result bodies WITHOUT
// waiting for an overflow. Chosen below pruneSufficientRatio (0.7, the "is a
// prune enough to retry on" test in conversation): by the time the reactive path
// would measure, the request has already been rejected once and every iteration
// up to it re-sent the full history.
const earlyPruneRatio = 0.55

// earlyPruneMinIter is how many iterations a turn must run before the early
// prune is considered at all. A short turn that is already large is large
// because of its OPENING context (a big paste, a long resumed transcript), not
// because of accumulated tool output — pruning there would drop the user's own
// material to no benefit.
const earlyPruneMinIter = 3

// maybeEarlyPrune drops the bodies of old, oversized tool results once the
// in-flight request crosses earlyPruneRatio of the model's context window,
// instead of waiting for the provider to reject the turn.
//
// Why this exists. pruneAndRetry (toolloop_prune.go) is reached only from
// decideRecovery's compact branch, which fires on errContextOverflow or
// StopContextWindow — i.e. AFTER a request was refused. Until then every
// iteration re-sends every prior tool result, so a 20-iteration turn ships the
// accumulated output ~20 times. The prompt cache absorbs much of that, but a
// batch of parallel tool calls can push the previous cached prefix past
// Anthropic's ~20-block lookup horizon (see the hedge breakpoint in
// providers/anthropic.go), and a miss there re-writes the whole history at full
// price. Pruning early is free — no summarizer call — and bounds the tail that
// a miss can cost.
//
// It runs at most ONCE per turn. Each prune rewrites history mid-loop, which
// invalidates the cached prefix from that point and, on preserved-thinking
// models, drops every thinking block after it (see thinkingdrop.go). One
// decisive pass pays that cost a single time; pruning a little each iteration
// would pay it every iteration and defeat the purpose.
//
// The reactive path is left exactly as it was: this never sets ls.compacted, so
// a genuine overflow still gets its prune-then-fold recovery. By then this pass
// has usually already taken the large bodies, and pruneAndRetry finds nothing
// left to drop and falls straight through to the fold — which is correct.
func (t *toolLoopTurn) maybeEarlyPrune(iter int) {
	if t.earlyPruned || iter < earlyPruneMinIter {
		return
	}
	// A CLI-wrapper provider owns the transcript; req.Messages is only the delta
	// it has not seen yet, so pruning our side cannot shrink what it holds. Same
	// exemption pruneAndRetry makes, for the same reason.
	if _, isCLI := providers.AsCLI(t.provider); isCLI {
		return
	}
	window := providers.ContextWindowFor(t.agent.Provider, t.agent.Model)
	if window <= 0 {
		return // unknown family: no ratio to measure against
	}
	// Tool schemas ride every request too, and in native-search mode they are the
	// whole deferred catalog — measuring messages alone would under-read the
	// footprint by exactly the term that makes a big catalog expensive.
	if conversation.EstimateInFlightTokens(t.req.Messages, t.req.Tools) < int(float64(window)*earlyPruneRatio) {
		return
	}
	pruned, stat, ok := conversation.PruneInFlightToolResultsMin(
		t.req.Messages, t.keepRecent, conversation.PruneMinBytesFor(window))
	if !ok {
		// Nothing qualified: the footprint is prose or already-pruned markers, and
		// re-measuring it every remaining iteration would buy nothing. Mark the
		// pass spent so this stays O(1) per turn.
		t.earlyPruned = true
		return
	}
	t.req.Messages = pruned
	t.earlyPruned = true
	// Visible like every other context reduction: a silent history rewrite is
	// exactly what the trace rules forbid. Same card and journal entry the
	// recovery-path prune emits, tagged with its own reason so the two are
	// distinguishable in debug.jsonl.
	pst := prunedToolResultsStep(stat)
	t.steps = append(t.steps, pst)
	t.emit(pst)
	t.r.emitDebug(t.ctx, prunedToolResultsEvent(t.agent.ID, reasonEarlyPrune, stat))
	t.r.logger.Info("pruned tool results early (pre-overflow)",
		"agent", t.agent.ID, "iter", iter, "pruned", stat.Pruned,
		"before_tokens", stat.BeforeTokens, "after_tokens", stat.AfterTokens, "window", window)
}
