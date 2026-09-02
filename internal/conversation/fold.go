package conversation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// errFoldSummary marks the one failure inside a rolling fold that must NOT kill
// the turn: the summarizer LLM call itself. Everything else the fold does (write
// the summary to the session, re-measure the persisted-step overhead) is local
// state the turn depends on, so those failures stay fatal and propagate.
//
// Prepare distinguishes the two with errors.Is. Wrapping rather than returning a
// second error keeps applyRollingFold's signature a plain (result, error).
var errFoldSummary = errors.New("rolling fold summary")

// rollingFoldInput carries what one rolling fold needs from Prepare. It is a
// struct rather than a dozen positional parameters because the accounting below
// mixes four different token figures (before, overhead, folded steps, after)
// whose order would be trivial to transpose at a call site.
type rollingFoldInput struct {
	database  *db.DB
	provider  providers.Provider
	session   db.Session
	agent     db.Agent
	history   []db.Message
	summary   string       // rolling summary as it stands BEFORE this fold
	fold      []db.Message // the messages going into the summary
	keepTail  []db.Message // the messages staying verbatim
	newCount  int          // new SummaryMsgCount watermark
	before    int          // message-only estimate that tripped the gate
	overhead  int          // non-message overhead as measured before the fold
	maxTokens int          // the budget, for logging/journalling only
}

// rollingFoldResult is the post-fold state Prepare adopts when the fold worked.
type rollingFoldResult struct {
	summary  string
	pending  []db.Message
	overhead int // overhead with the folded messages' persisted-step term removed
	stat     Compaction
}

// applyRollingFold performs one rolling-summary fold: summarize the folded
// messages, persist the new summary, and re-measure the turn's footprint. It is
// the body Prepare used to inline; it moved here so Prepare can treat a failed
// summarizer call as a recoverable condition without burying the accounting in
// another level of nesting.
//
// The token accounting is unchanged from the inline version and deliberately
// asymmetric — see the comments on each side of the deduction.
func (m *Manager) applyRollingFold(ctx context.Context, in rollingFoldInput) (rollingFoldResult, error) {
	// PreCompact lifecycle hook seam: fire before the fold runs (Claude Code
	// parity). "auto" = the routine budgeted fold (manual /compact passes
	// "manual" via its own path).
	firePreCompact(ctx, TriggerAuto)
	summary, err := m.summarizeTimed(ctx, in.database, in.provider, in.agent, in.session.ID, in.summary, in.fold)
	if err != nil {
		return rollingFoldResult{}, fmt.Errorf("%w: %v", errFoldSummary, err)
	}
	foldIndex, err := in.database.SetSessionSummary(ctx, in.session.ID, summary, in.newCount)
	if err != nil {
		return rollingFoldResult{}, err
	}
	pending := in.keepTail
	// The overhead was measured BEFORE the fold, so its persisted-Steps term
	// still charges the trace of the messages just folded away. Drop that part
	// (an error here is propagated, never counted as zero) so the reported
	// footprint — and the pressure ratio in Prepare — describe the post-fold turn.
	foldedSteps, err := foldedStepOverhead(ctx, in.history, in.newCount)
	if err != nil {
		return rollingFoldResult{}, fmt.Errorf("post-fold overhead: %w", err)
	}
	// Keep the pre-deduction figure: "before" must describe the footprint as it
	// stood when the gate fired, which still carried the folded trace. Reporting
	// both sides off the reduced overhead hides exactly the part the fold removed,
	// understating the "X→Y" ratio by foldedSteps.
	overheadBefore := in.overhead
	overhead := in.overhead - foldedSteps
	if overhead < 0 {
		overhead = 0 // a step term larger than the whole overhead is nonsense; floor it
	}
	afterTokens := EstimateTokens(summary, pending) + overhead
	// The on-screen compaction step and the debug journal share one formula:
	// messages + the non-message overhead as it stands AT THAT MOMENT — the
	// pre-fold overhead on the before side, the post-deduction one after.
	stat := Compaction{
		FoldedMsgs:   len(in.fold),
		BeforeTokens: in.before + overheadBefore,
		AfterTokens:  afterTokens,
		Trigger:      TriggerAuto,
		Mode:         ModeRolling,
	}
	m.log(slog.LevelInfo, "context compacted (rolling summary fold)",
		"session", in.session.ID, "agent", in.agent.ID,
		"folded_msgs", len(in.fold), "before_tokens", in.before, "overhead_tokens", overhead,
		"after_tokens", afterTokens, "budget", in.maxTokens)
	// Journal the fold to debug.jsonl (true footprint = messages + overhead, the
	// same basis the fold gate uses).
	m.recordCompactionDebug(in.database, in.session.ID, in.agent.ID, stat.Trigger,
		stat.FoldedMsgs, stat.BeforeTokens, stat.AfterTokens, in.maxTokens, len(renderDBMessages(in.fold)),
		foldIndex, len(summary))
	return rollingFoldResult{summary: summary, pending: pending, overhead: overhead, stat: stat}, nil
}
