package conversation

import (
	"context"
	"errors"
	"log/slog"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// nativeCompactCtxKey carries a callback that asks the ACTIVE CLI provider to
// compact its own context window — the seam the API layer uses to run native
// compaction from inside Prepare without conversation importing api (which would
// cycle). Modelled on preCompactCtxKey above it.
type nativeCompactCtxKey struct{}

// NativeCompactResult reports whether the invocation's native-success debug
// record was durably appended. The automatic gate uses this exact invocation
// result instead of counting a prunable, concurrently-written journal.
type NativeCompactResult struct {
	SuccessDebugPersisted bool
}

// errNoNativeCompactor is what fireNativeCompact reports when nobody installed a
// callback (every direct/test caller of Prepare, and every non-CLI turn path).
// Like any other non-nil result it simply means "native compaction did not
// happen", so the caller falls back to the rolling fold.
var errNoNativeCompactor = errors.New("no native compaction callback on context")

// WithNativeCompact returns a context carrying the native-compaction callback the
// automatic gate invokes before it would fold history into the rolling summary.
// The callback returns nil when the provider actually compacted its window, and
// any error when it could not (unsupported provider, no resumable CLI thread, a
// failed call). conversation deliberately does NOT classify the error — the
// unavailability sentinel lives in the api package — so the rule here is simply
// nil = native happened, non-nil = fall back to the rolling fold.
// nil fn is a no-op.
func WithNativeCompact(ctx context.Context, fn func(context.Context) (NativeCompactResult, error)) context.Context {
	if fn == nil {
		return ctx
	}
	return context.WithValue(ctx, nativeCompactCtxKey{}, fn)
}

// fireNativeCompact invokes the ctx-carried native-compaction callback, or reports
// errNoNativeCompactor when there is none.
func fireNativeCompact(ctx context.Context) (NativeCompactResult, error) {
	fn, ok := ctx.Value(nativeCompactCtxKey{}).(func(context.Context) (NativeCompactResult, error))
	if !ok || fn == nil {
		return NativeCompactResult{}, errNoNativeCompactor
	}
	return fn(ctx)
}

// claimNativeAttempt is the anti-loop gate in front of native compaction.
//
// Native compaction shrinks the CLI's OWN window; it does not shrink the pending
// transcript TionHarness estimates, so EstimateTokens(summary, pending) is exactly
// the same on the next turn. Without a guard the gate would therefore see the same
// over-budget footprint every single turn and fire another native compaction (a CLI
// round-trip plus a dropped warm session) forever, never converging.
//
// The guard: a session may attempt native compaction only once per rolling-summary
// boundary. New user/assistant messages grow history without reducing our pending
// transcript, so history length cannot prove progress. Only a later rolling fold,
// observed as a larger SummaryMsgCount, permits another native attempt. Thus
// persistently over-budget turns alternate native then rolling and converge.
//
// The attempt is claimed before the callback runs: a failed native attempt falls
// back to rolling on that same turn, so re-claiming it would buy nothing.
func (m *Manager) claimNativeAttempt(sessionID string, summaryMsgCount int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if last, ok := m.lastNativeCompactAt[sessionID]; ok && summaryMsgCount <= last {
		return false
	}
	if m.lastNativeCompactAt == nil {
		m.lastNativeCompactAt = map[string]int{}
	}
	m.lastNativeCompactAt[sessionID] = summaryMsgCount
	return true
}

// nativeCompactErrorKind classifies why native compaction did not happen, using
// only reason enums the debug journal keeps verbatim. errNoNativeCompactor is
// absence rather than failure — no CLI callback was installed for this turn — so
// it carries no kind at all.
func nativeCompactErrorKind(err error) string {
	if err == nil || errors.Is(err, errNoNativeCompactor) {
		return ""
	}
	return "compaction_failed"
}

// recordNativeCompactDebug journals the native-compaction decisions that leave no
// other trace: the attempt claimNativeAttempt refused because the claim for this
// rolling-summary boundary was already spent (native_skipped), the claim a failed
// attempt consumed (claim_consumed), and the automatic mode's fall-through to the
// rolling fold (native_fallback_rolling). Only the SUCCESSFUL native path had a
// record before, so the journal showed a rolling fold with no explanation of why
// native had not run. Same gating and failure handling as the other recorders
// here: a blank session id or a nil store writes nothing, an append error is
// logged rather than swallowed.
func (m *Manager) recordNativeCompactDebug(database *db.DB, sessionID, agentID, name, errorKind string) {
	if database == nil || sessionID == "" {
		return
	}
	if err := database.AppendDebugEventGated(sessionID, db.DebugEvent{
		Type:      db.DebugCompaction,
		AgentID:   agentID,
		Name:      name,
		ErrorKind: errorKind,
	}); err != nil {
		m.log(slog.LevelError, "native compaction decision journal append failed",
			"session", sessionID, "name", name, "error", err)
	}
}
