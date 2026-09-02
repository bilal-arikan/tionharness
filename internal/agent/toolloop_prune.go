package agent

import (
	"github.com/bilal-arikan/tionharness/internal/conversation"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// pruneAndRetry is phase 1 of overflow recovery: drop the bodies of old,
// oversized tool results and retry WITHOUT a summarizer call. It returns true
// when the retry should proceed on the pruned history alone.
//
// It deliberately does NOT set ls.compacted. The one-shot budget in
// decideRecovery guards the expensive fold, and a prune is free; leaving the
// flag clear means a second overflow still gets its fold — by then the prune
// finds nothing left to drop and falls through to it immediately.
//
// It also leaves req.ResumeSessionID alone. Clearing it is how the fold hands a
// CLI provider a fresh session seeded with the summary; there is no summary
// here, so clearing it would throw the CLI's history away and replace it with
// nothing.
func (t *toolLoopTurn) pruneAndRetry(reason contReason) bool {
	// A CLI-wrapper provider owns the transcript that overflowed — req.Messages
	// is only the delta it has not seen yet (see the claude-cli resume path), so
	// pruning our side cannot shrink what actually blew the window. Go straight
	// to the fold, which restarts the CLI session from a summary.
	if _, isCLI := providers.AsCLI(t.provider); isCLI {
		return false
	}
	pruned, stat, ok := conversation.PruneInFlightToolResults(t.req.Messages, t.keepRecent)
	if !ok {
		return false
	}
	t.req.Messages = pruned
	// Report the prune even when it turns out to be insufficient: it changed the
	// history the model sees, and a silent rewrite is exactly what the visibility
	// rule forbids. The fold that may follow adds its own pair of cards.
	pst := prunedToolResultsStep(stat)
	t.steps = append(t.steps, pst)
	t.emit(pst)
	t.r.emitDebug(t.ctx, prunedToolResultsEvent(t.agent.ID, string(reason), stat))
	if !conversation.PruneSufficient(stat, providers.ContextWindowFor(t.agent.Provider, t.agent.Model)) {
		return false
	}
	t.ls.lastContinue = reason
	// Same signal the fold raises: the turn hit the context limit, so an
	// autonomous caller can still decide on a context-reset handoff.
	markContextOverflow(t.ctx)
	rec := TurnStep{Kind: StepRecovery, Reason: string(reason), Text: recoveryText(reason)}
	t.steps = append(t.steps, rec)
	t.emit(rec)
	return true
}
