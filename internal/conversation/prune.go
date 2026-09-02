package conversation

import (
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

// Phase-1 context reduction: drop the BODIES of old, oversized tool results
// from an in-flight message slice. It costs nothing — no summarizer call, no
// provider round-trip — and is therefore tried BEFORE the reactive fold
// (reactive.go), which spends a real LLM call to compress the same history.
//
// Scope note. TionHarness only carries tool results inside a single turn: the
// stored transcript keeps them as db.Message.Steps for display, and Prepare
// rebuilds provider messages from Text alone (see toProviderMessages), so a
// tool result never survives into the NEXT turn. The bloat this addresses is
// therefore always in-flight: one long tool loop whose accumulated outputs
// overflow the window. That is the same place CompactInFlightMessages runs.
//
// The transformation is deliberately narrow: only ToolResult.Content changes.
// CallID, IsError and the surrounding message structure are left exactly as
// they were, so the tool_use↔tool_result pairing RepairSequence enforces still
// holds after a prune.

// pruneToolResultMinBytes is the size above which an old tool result's body is
// replaced by a marker. Below it a prune saves almost nothing while still
// costing the model a fact it could have used, so small results are left alone.
// 4 KB is well under the 100 KB write-time cap (MaxToolOutputKB) and well above
// an ordinary command's output.
const pruneToolResultMinBytes = 4096

// pruneSufficientRatio is the share of the model's context window the pruned
// history must fit under for the caller to retry WITHOUT also paying for a
// summarizer fold. Deliberately well below 1.0: the estimate is a heuristic
// (see charsPerToken) and the request also carries the system prompt and tool
// schemas, neither of which is in EstimateProviderTokens.
const pruneSufficientRatio = 0.7

// prunedToolResultPrefix opens every marker. It is also the idempotency guard:
// a second prune pass recognises its own output and leaves it alone instead of
// re-wrapping it.
const prunedToolResultPrefix = "[tool result pruned"

// PruneStat reports what one prune pass did, in the same before/after token
// vocabulary the folds use, so the on-screen card and the debug journal can be
// built from it without recomputing anything.
type PruneStat struct {
	Pruned       int // tool results whose body was replaced
	BeforeTokens int // estimated tokens of the slice before the prune
	AfterTokens  int // estimated tokens after
	SavedBytes   int // raw bytes of tool output dropped
}

// PruneInFlightToolResults replaces the body of every tool result older than
// the kept tail and larger than pruneToolResultMinBytes with a one-line marker
// naming what was dropped. ok=false means nothing qualified and msgs is
// returned unchanged — the caller must then fall through to its normal
// (summarizer-backed) compaction.
//
// The input slice is never mutated: a message whose results change is rewritten
// in a copy, so a caller holding the original (e.g. an askSuspend payload
// captured earlier in the turn) keeps the history it captured.
func PruneInFlightToolResults(msgs []providers.Message, keepRecent int) ([]providers.Message, PruneStat, bool) {
	if keepRecent < 1 {
		keepRecent = 1
	}
	limit := len(msgs) - keepRecent
	if limit <= 0 {
		return msgs, PruneStat{}, false
	}
	out := msgs
	cloned := false
	stat := PruneStat{}
	for i := 0; i < limit; i++ {
		src := msgs[i]
		if len(src.ToolResults) == 0 {
			continue
		}
		// A message carrying RawContent is echoed to the provider VERBATIM
		// (see providers.Message.RawContent), so its ToolResults never reach the
		// wire. Pruning them would report a saving the model never sees.
		if len(src.RawContent) > 0 {
			continue
		}
		var results []providers.ToolResult
		for j, tr := range src.ToolResults {
			if len(tr.Content) <= pruneToolResultMinBytes || strings.HasPrefix(tr.Content, prunedToolResultPrefix) {
				continue
			}
			if results == nil {
				if !cloned {
					out = append([]providers.Message(nil), msgs...)
					cloned = true
				}
				results = append([]providers.ToolResult(nil), src.ToolResults...)
			}
			marker := prunedToolResultMarker(tr.Content)
			stat.SavedBytes += len(tr.Content) - len(marker)
			stat.Pruned++
			results[j].Content = marker
		}
		if results != nil {
			out[i].ToolResults = results
		}
	}
	if stat.Pruned == 0 {
		return msgs, PruneStat{}, false
	}
	stat.BeforeTokens = EstimateProviderTokens(msgs)
	stat.AfterTokens = EstimateProviderTokens(out)
	return out, stat, true
}

// prunedToolResultMarker renders what replaced a dropped body. It names the
// size and line count that were there and says plainly that the output is gone,
// so the model re-runs the tool instead of answering from a body it can no
// longer read — a silent empty result would invite exactly that mistake.
func prunedToolResultMarker(content string) string {
	lines := strings.Count(content, "\n") + 1
	return fmt.Sprintf("%s to fit the context window — %s, %d lines dropped. The full output is no longer in this turn's history; re-run the tool if you still need it.]",
		prunedToolResultPrefix, humanSize(int64(len(content))), lines)
}

// PruneSufficient reports whether a pruned history is small enough to retry the
// provider call WITHOUT also folding it through the summarizer.
//
// window<=0 (model window unknown) answers true: the prune already happened and
// cost nothing, the retry costs one call either way, and the summarizer fold
// stays available for the next overflow because the caller only marks the turn
// compacted when it actually folds. Spending a summarizer call on a guess is
// the worse trade.
func PruneSufficient(stat PruneStat, window int) bool {
	if window <= 0 {
		return true
	}
	return stat.AfterTokens <= int(float64(window)*pruneSufficientRatio)
}
