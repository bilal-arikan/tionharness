package conversation

import (
	"context"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
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
func CompactInFlightMessages(ctx context.Context, database *db.DB, provider providers.Provider, agent db.Agent, msgs []providers.Message, keepRecent int) (out []providers.Message, ok bool, err error) {
	if keepRecent < 1 {
		keepRecent = 1
	}
	limit := len(msgs) - keepRecent
	if limit <= 0 {
		return msgs, false, nil
	}
	cut := -1
	for i := limit; i >= 1; i-- {
		if msgs[i].Role == providers.RoleAssistant {
			cut = i
			break
		}
	}
	if cut <= 0 {
		return msgs, false, nil
	}
	summary, err := summarizeProviderMessages(ctx, database, provider, agent, msgs[:cut])
	if err != nil {
		return nil, false, err
	}
	out = make([]providers.Message, 0, len(msgs)-cut+1)
	out = append(out, providers.Message{
		Role: providers.RoleUser,
		Text: "Summary of earlier conversation:\n" + summary,
	})
	out = append(out, msgs[cut:]...)
	return out, true, nil
}

// summarizeProviderMessages folds in-flight provider messages into a single
// summary via the shared compaction core. It has no prior rolling summary (the
// loop's messages are transient), so existing is "". renderProviderMessages adds a
// terse note of any tool calls/results so the summary keeps that signal.
func summarizeProviderMessages(ctx context.Context, database *db.DB, provider providers.Provider, agent db.Agent, msgs []providers.Message) (string, error) {
	return summarizeRendered(ctx, database, provider, agent, "", renderProviderMessages(msgs))
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
