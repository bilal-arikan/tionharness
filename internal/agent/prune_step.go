package agent

import (
	"fmt"

	"github.com/bilal-arikan/tionharness/internal/conversation"
	"github.com/bilal-arikan/tionharness/internal/db"
)

// prunedToolResultsStep builds the on-screen card for a phase-1 prune — the
// LLM-less pass that drops old oversized tool-result bodies before the reactive
// fold is attempted. It reuses the compaction card (this IS a context
// reduction, and the user cares about the same before/after figures) but leaves
// FoldedMsgs at 0 and tags the trigger "prune": nothing was summarized, so
// claiming a folded message count would be a lie.
func prunedToolResultsStep(stat conversation.PruneStat) TurnStep {
	return TurnStep{
		Kind: StepCompaction,
		Text: fmt.Sprintf("🧹 Eski araç çıktıları budandı — %d sonuç kısaltıldı (%d→%d token).",
			stat.Pruned, stat.BeforeTokens, stat.AfterTokens),
		BeforeTokens: stat.BeforeTokens,
		AfterTokens:  stat.AfterTokens,
		Trigger:      conversation.TriggerPrune,
		Source:       "tionharness",
	}
}

// prunedToolResultsEvent builds the debug-journal entry for the same prune.
// SavedBytes carries the raw tool output dropped; SummaryBytes is deliberately
// absent because no summary was produced.
func prunedToolResultsEvent(agentID, reason string, stat conversation.PruneStat) db.DebugEvent {
	return db.DebugEvent{
		Type:    db.DebugCompaction,
		AgentID: agentID,
		Name:    conversation.TriggerPrune,
		Detail: fmt.Sprintf("%s · pruned %d tool results · %d→%d tokens · no summarizer call",
			reason, stat.Pruned, stat.BeforeTokens, stat.AfterTokens),
		SavedBytes: stat.SavedBytes,
	}
}
