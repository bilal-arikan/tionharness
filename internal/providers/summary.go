package providers

import "strings"

// joinNonEmpty concatenates the trimmed, non-empty blocks with a blank-line
// separator, dropping empties. Used to fold the rolling summary back into the
// dynamic/system prompt on providers (or code paths) that do not place it as a
// separate cached head message.
func joinNonEmpty(blocks ...string) string {
	parts := make([]string, 0, len(blocks))
	for _, b := range blocks {
		if s := strings.TrimSpace(b); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "\n\n")
}

// prependSummaryMessage inserts the rolling compaction summary as a synthetic
// head user message so it becomes part of the cached prompt prefix — BEFORE the
// rolling history breakpoint the native providers place on the last message.
// This is TionHarness's take on Claude Code's "compact boundary message": the
// summary is stable between two folds, so once it sits in the cached prefix it is
// a cache READ every turn until the next fold rewrites it, instead of being
// re-shipped as fresh input tokens each turn.
//
// The head message is safe for strict-alternation providers: the first live
// message after a fold is never an orphaned tool_result user turn (its matching
// tool_use assistant turn would precede it, so it could not be first), so it is
// either an assistant turn (→ clean user→assistant alternation) or a genuine user
// turn (→ merged into one user message by the provider's same-role coalescing;
// anthropic does it block-wise in toAnthropicMessages). The returned
// slice is a fresh backing array, so the caller's Messages are never mutated (the
// summary head is request-time only and must never be persisted).
func prependSummaryMessage(msgs []Message, summary string) []Message {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return msgs
	}
	out := make([]Message, 0, len(msgs)+1)
	out = append(out, Message{Role: RoleUser, Text: summary})
	out = append(out, msgs...)
	return out
}
