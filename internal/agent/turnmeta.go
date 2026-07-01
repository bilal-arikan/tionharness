package agent

import (
	"context"

	"github.com/bilal-arikan/swarmgo/internal/db"
	"github.com/bilal-arikan/swarmgo/internal/providers"
)

// turnMeta captures the provider-side metadata of a completed turn (the model
// that answered, why generation ended, and token usage) so an AUTONOMOUS caller
// (scheduler / spawn / inbox / flow) can stamp it onto the assistant message it
// persists — the same per-bubble enrichment the chat paths set inline from the
// response. It is carried by pointer in ctx so the wrappers (invokeTraced /
// runSessionTurn / complete) don't need new return values: completeTraced writes
// the final completion's values here, the caller reads them after.
type turnMeta struct {
	Model      string
	StopReason string
	Usage      providers.Usage
}

type turnMetaKey struct{}

// WithTurnMeta attaches a fresh, empty turn-meta sink to ctx and returns it. The
// autonomous caller wraps its turn ctx with this, then reads the populated value
// when building the assistant message.
func WithTurnMeta(ctx context.Context) (context.Context, *turnMeta) {
	m := &turnMeta{}
	return context.WithValue(ctx, turnMetaKey{}, m), m
}

// turnMetaFrom returns the sink attached to ctx, or nil when none (chat turns,
// which set these fields inline from the response, don't install one).
func turnMetaFrom(ctx context.Context) *turnMeta {
	m, _ := ctx.Value(turnMetaKey{}).(*turnMeta)
	return m
}

// capture records a completion's metadata. Called by completeTraced on every
// successful provider call, so the LAST call of a tool-loop turn wins (matching
// the chat paths, which persist the final response's usage/model/stop reason).
func (m *turnMeta) capture(resp *providers.Response) {
	if m == nil || resp == nil {
		return
	}
	if resp.Model != "" {
		m.Model = resp.Model
	}
	m.StopReason = resp.StopReason
	m.Usage = resp.Usage
}

// apply stamps the captured metadata (plus the caller-measured duration) onto an
// assistant message before it is persisted. Nil-safe: a turn with no meta sink
// just leaves the enrichment fields empty (omitempty → absent on disk).
func (m *turnMeta) apply(msg *db.Message, durMs int64) {
	if durMs > 0 {
		msg.DurationMs = durMs
	}
	if m == nil {
		return
	}
	msg.Model = m.Model
	msg.StopReason = m.StopReason
	msg.Usage = usageMsg(m.Usage)
}

// sumUsage adds two provider Usage values field-by-field. Used by the native tool
// loop to accumulate every iteration's tokens into one turn total, so the persisted
// assistant bubble matches what RecordUsage summed into the daily/session rollups.
func sumUsage(a, b providers.Usage) providers.Usage {
	return providers.Usage{
		InputTokens:      a.InputTokens + b.InputTokens,
		OutputTokens:     a.OutputTokens + b.OutputTokens,
		CacheReadTokens:  a.CacheReadTokens + b.CacheReadTokens,
		CacheWriteTokens: a.CacheWriteTokens + b.CacheWriteTokens,
	}
}

// usageMsg converts a provider Usage into the compact per-message form, returning
// nil when the turn reported no tokens (so a non-LLM turn carries no usage object).
// The agent-package twin of api.messageUsage.
func usageMsg(u providers.Usage) *db.MessageUsage {
	if u.InputTokens == 0 && u.OutputTokens == 0 && u.CacheReadTokens == 0 && u.CacheWriteTokens == 0 {
		return nil
	}
	return &db.MessageUsage{
		InputTokens:      u.InputTokens,
		OutputTokens:     u.OutputTokens,
		CacheReadTokens:  u.CacheReadTokens,
		CacheWriteTokens: u.CacheWriteTokens,
	}
}
