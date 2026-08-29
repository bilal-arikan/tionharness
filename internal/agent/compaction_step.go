package agent

import (
	"fmt"

	"github.com/bilal-arikan/tionharness/internal/conversation"
	"github.com/bilal-arikan/tionharness/internal/db"
)

// reactiveCompactionStep builds the on-screen card for a REACTIVE fold — the
// mid-turn compaction the tool loop performs when a provider call overflows the
// context window. It mirrors the interactive/autonomous compaction step
// (api.compactionLeadStep) so a fold renders identically no matter which path
// produced it; only the trigger and the wording differ ("mid-turn" instead of
// "otomatik"). It does NOT replace the recovery step emitted alongside it: the
// recovery card explains why the turn was retried, this one what the retry cost.
func reactiveCompactionStep(fold conversation.ReactiveFold) TurnStep {
	return TurnStep{
		Kind:         StepCompaction,
		Text:         fmt.Sprintf("🗜 Bağlam tur içinde sıkıştırıldı — %d mesaj özete katlandı (%d→%d token).", fold.FoldedMsgs, fold.BeforeTokens, fold.AfterTokens),
		FoldedMsgs:   fold.FoldedMsgs,
		BeforeTokens: fold.BeforeTokens,
		AfterTokens:  fold.AfterTokens,
		Trigger:      fold.Trigger,
	}
}

// reactiveCompactionEvent builds the debug-journal entry for the same fold, with
// the real byte and token figures instead of the bare reason string it used to
// carry. FoldIndex is deliberately absent: an in-flight fold never writes the
// session summary, so it has no ordinal in the session's fold series — Detail
// says so rather than journalling a misleading fold #0.
func reactiveCompactionEvent(agentID, reason string, fold conversation.ReactiveFold) db.DebugEvent {
	return db.DebugEvent{
		Type:    db.DebugCompaction,
		AgentID: agentID,
		Name:    fold.Trigger,
		Detail: fmt.Sprintf("%s · folded %d msgs · %d→%d tokens · fold # n/a (in-flight)",
			reason, fold.FoldedMsgs, fold.BeforeTokens, fold.AfterTokens),
		SavedBytes:   fold.SavedBytes,
		SummaryBytes: fold.SummaryBytes,
	}
}
