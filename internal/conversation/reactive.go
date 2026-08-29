package conversation

import (
	"context"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// CompactInFlightMessages folds the older portion of an in-flight provider
// message slice into a single summary, for reactive mid-loop recovery when a
// turn overflows the context window. Unlike Manager.ForceCompact it never
// touches the DB session — the loop's request messages are transient — so it is
// a package-level function the agent loop can call directly without holding a
// Manager.
//
// The fold boundary is chosen at an assistant message at or before
// len(msgs)-keepRecent, so (1) the kept tail begins with an assistant turn,
// preserving user→assistant role alternation after the summary user-message is
// prepended, and (2) no assistant tool_use is separated from its following
// tool_result. When no safe boundary exists (history too short, or all foldable
// turns are user turns) it folds nothing and reports ok=false so the caller can
// fall back to surfacing the original error.
//
// The returned ReactiveFold carries the figures the caller needs to make the
// fold visible (on-screen compaction step) and observable (debug journal) —
// without them a reactive fold silently rewrites the turn's history.
func CompactInFlightMessages(ctx context.Context, database *db.DB, provider providers.Provider, agent db.Agent, msgs []providers.Message, keepRecent int) (out []providers.Message, fold ReactiveFold, ok bool, err error) {
	if keepRecent < 1 {
		keepRecent = 1
	}
	limit := len(msgs) - keepRecent
	if limit <= 0 {
		return msgs, ReactiveFold{}, false, nil
	}
	cut := -1
	for i := limit; i >= 1; i-- {
		if msgs[i].Role == providers.RoleAssistant {
			cut = i
			break
		}
	}
	if cut <= 0 {
		return msgs, ReactiveFold{}, false, nil
	}
	// Rendered here (not inside the summarizer) so its byte size can be reported
	// as SavedBytes — the same basis the rolling fold journals.
	rendered := renderProviderMessages(msgs[:cut])
	before := EstimateProviderTokens(msgs)
	summary, err := summarizeRendered(ctx, database, provider, agent, "", rendered)
	if err != nil {
		return nil, ReactiveFold{}, false, err
	}
	out = make([]providers.Message, 0, len(msgs)-cut+1)
	out = append(out, providers.Message{
		Role: providers.RoleUser,
		Text: "Summary of earlier conversation:\n" + summary,
	})
	out = append(out, msgs[cut:]...)
	fold = ReactiveFold{
		Compaction: Compaction{
			FoldedMsgs:   cut,
			BeforeTokens: before,
			AfterTokens:  EstimateProviderTokens(out),
			Trigger:      TriggerReactive,
		},
		SavedBytes:   len(rendered),
		SummaryBytes: len(summary),
	}
	return out, fold, true, nil
}

// ReactiveFold describes one in-flight fold: the shared Compaction figures the
// on-screen step renders, plus the byte figures the debug journal records. It is
// NOT a session fold — nothing is written to the session summary — so it has no
// fold ordinal (FoldIndex); the journal says so instead of inventing one.
type ReactiveFold struct {
	Compaction
	SavedBytes   int // rendered size of the folded messages
	SummaryBytes int // size of the summary that replaced them
}

// EstimateProviderTokens approximates the token footprint of an in-flight
// provider message slice. It counts tool-call arguments and tool results too —
// on the loop's history those routinely outweigh the prose, and the reactive
// fold exists precisely because that history overflowed.
func EstimateProviderTokens(msgs []providers.Message) int {
	total := 0
	for _, m := range msgs {
		total += estimateText(m.Text) + MsgOverhead
		for _, tc := range m.ToolCalls {
			total += estimateText(tc.Name) + estimateText(string(tc.Input))
		}
		for _, tr := range m.ToolResults {
			total += estimateText(tr.Content)
		}
	}
	return total
}

// renderProviderMessages flattens provider messages to the transcript the
// compaction prompt expects: each turn's text plus a terse marker for any tool
// calls and tool results.
func renderProviderMessages(msgs []providers.Message) string {
	var b strings.Builder
	for _, m := range msgs {
		b.WriteString(m.Role)
		b.WriteString(": ")
		if m.Text != "" {
			b.WriteString(m.Text)
		}
		for _, tc := range m.ToolCalls {
			b.WriteString("\n[tool call: ")
			b.WriteString(tc.Name)
			b.WriteString("]")
		}
		for range m.ToolResults {
			b.WriteString("\n[tool result]")
		}
		b.WriteString("\n")
	}
	return b.String()
}
