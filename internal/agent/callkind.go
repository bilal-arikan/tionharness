package agent

import (
	"context"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/tools"
)

// CallKind tags a provider call by its origin so usage can be attributed across
// every entry point — chat, autonomous task/schedule/heartbeat, flow nodes,
// agent→agent delegation, and the auxiliary calls (titling, summaries,
// reflection, context compaction). It mirrors the db.UsageKind* taxonomy.
type CallKind string

const (
	KindChat      CallKind = db.UsageKindChat
	KindTask      CallKind = db.UsageKindTask
	KindSchedule  CallKind = db.UsageKindSchedule
	KindFlow      CallKind = db.UsageKindFlow
	KindHeartbeat CallKind = db.UsageKindHeartbeat
	KindDelegate  CallKind = db.UsageKindDelegate
	KindSpawn     CallKind = db.UsageKindSpawn
	KindTitle     CallKind = db.UsageKindTitle
	KindSummary   CallKind = db.UsageKindSummary
	KindReflect   CallKind = db.UsageKindReflect
	KindCompact   CallKind = db.UsageKindCompact
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

// callKindFrom returns the call origin stamped on the context, defaulting to
// KindChat for user-initiated chat turns that don't stamp one explicitly.
func callKindFrom(ctx context.Context) CallKind {
	if k, ok := ctx.Value(callKindKey{}).(CallKind); ok && k != "" {
		return k
	}
	return KindChat
}
