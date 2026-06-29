package agent

import (
	"context"
	"sync/atomic"

	"github.com/bilal-arikan/swarmgo/internal/db"
	"github.com/bilal-arikan/swarmgo/internal/tools"
)

// CallKind tags a provider call by its origin so usage can be attributed across
// every entry point — chat, autonomous task/schedule, flow nodes,
// agent→agent delegation, and the auxiliary calls (titling, summaries,
// reflection, context compaction). It mirrors the db.UsageKind* taxonomy.
type CallKind string

const (
	KindChat     CallKind = db.UsageKindChat
	KindTask     CallKind = db.UsageKindTask
	KindSchedule CallKind = db.UsageKindSchedule
	KindFlow     CallKind = db.UsageKindFlow
	KindDelegate CallKind = db.UsageKindDelegate
	KindSpawn    CallKind = db.UsageKindSpawn
	KindSubagent CallKind = db.UsageKindSubagent
	KindTitle    CallKind = db.UsageKindTitle
	KindSummary  CallKind = db.UsageKindSummary
	KindReflect  CallKind = db.UsageKindReflect
	KindCompact  CallKind = db.UsageKindCompact
)

type callKindKey struct{}

// WithCallKind stamps a call origin onto the context. Every provider call made
// downstream — including the inner tool-loop iterations and any context
// compaction triggered mid-turn — is attributed to this kind. Stamp it once at
// each origin entry point; the default (unstamped) kind is KindChat.
//
// Non-chat kinds also stamp the context as autonomous so interactive-only tools
// (ask_user, request_confirmation) can bail out immediately without parsing
// the input payload.
func WithCallKind(ctx context.Context, kind CallKind) context.Context {
	ctx = context.WithValue(ctx, callKindKey{}, kind)
	if kind != KindChat {
		ctx = tools.WithAutonomous(ctx)
	}
	return ctx
}

type sessionIDKey struct{}

// WithSessionID stamps the originating session id onto the context. The
// autonomous Interaction MCP wiring (scheduler/spawn) reads it so a
// CLI agent's schedule_wake targets the right session. Optional: an unstamped
// context yields "" (wake then degrades to unavailable, not wrong-session).
func WithSessionID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, sessionIDKey{}, id)
}

// SessionIDFrom returns the session id stamped on the context, or "" if none.
func SessionIDFrom(ctx context.Context) string {
	if id, ok := ctx.Value(sessionIDKey{}).(string); ok {
		return id
	}
	return ""
}

type turnIDKey struct{}

// WithTurnID stamps the in-flight assistant reply message id onto the context, so
// every debug event emitted during the turn (llm_call, tool, hook, error,
// recovery, compaction) can be correlated back to THAT chat message for the
// per-message debug panel. Stamp it once per turn at the chat entry point.
func WithTurnID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, turnIDKey{}, id)
}

// TurnIDFrom returns the reply message id stamped on the context, or "" if none.
func TurnIDFrom(ctx context.Context) string {
	if id, ok := ctx.Value(turnIDKey{}).(string); ok {
		return id
	}
	return ""
}

type overflowKey struct{}

// withOverflowFlag attaches a context-overflow flag to the context and returns
// it alongside the flag. An autonomous caller installs it before a turn so that,
// after the turn, it can tell whether the tool loop had to reactively compact its
// in-flight history (the SwarmGo signal that the turn ran up against the context
// limit) — the trigger for an automatic context-reset handoff.
func withOverflowFlag(ctx context.Context) (context.Context, *atomic.Bool) {
	var flag atomic.Bool
	return context.WithValue(ctx, overflowKey{}, &flag), &flag
}

// markContextOverflow sets the overflow flag if one is installed on the context.
// Called by the tool loop when it performs a reactive in-flight compaction.
func markContextOverflow(ctx context.Context) {
	if f, ok := ctx.Value(overflowKey{}).(*atomic.Bool); ok && f != nil {
		f.Store(true)
	}
}

// callKindFrom returns the call origin stamped on the context, defaulting to
// KindChat for user-initiated chat turns that don't stamp one explicitly.
func callKindFrom(ctx context.Context) CallKind {
	if k, ok := ctx.Value(callKindKey{}).(CallKind); ok && k != "" {
		return k
	}
	return KindChat
}
