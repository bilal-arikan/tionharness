package agent

import (
	"context"

	"github.com/bilal-arikan/tionharness/internal/events"
	"github.com/bilal-arikan/tionharness/internal/turnqueue"
)

// turnslot.go is the runtime's face onto the per-session turn admission queue
// (internal/turnqueue). The queue itself is layer-free — it only orders turns; the
// wrappers here name the callers so the UI can say WHAT is waiting
// ("zamanlanmış tur", "otomasyon tetiği") rather than just "meşgul".
//
// Every turn-entry path in the process goes through one of these:
//
//	BeginSessionUserTurn                        → a user message (interactive or queued)
//	ClaimSessionCommandTurn                      → /compact, /handoff
//	claimSessionTurnSlotCtx(kind)                → wake, spawn, automation
//
// There is no priority: whoever asked first runs first. A priority scheme is
// exactly how autonomous auto-turns used to starve a waiting user message
// (_Docs/58).

// TurnQueue exposes the admission queue so the API layer can render it (what holds
// the session and what is queued behind) and so tests can observe it.
func (r *Runtime) TurnQueue() *turnqueue.Queue { return r.turns }

// BeginSessionUserTurn claims the session's turn slot for an interactive
// (user-initiated) turn so it never overlaps ANY other turn on the same session.
// Blocks until the slot is free; the returned release func MUST be deferred.
func (r *Runtime) BeginSessionUserTurn(sessionID string) (release func()) {
	rel, _ := r.claimTurnSlot(context.Background(), sessionID, turnqueue.KindUser, "kullanıcı mesajı")
	return rel
}

// ClaimSessionCommandTurn claims the slot for an out-of-queue slash command
// (/compact, /handoff) that runs a direct provider call on the HTTP goroutine.
func (r *Runtime) ClaimSessionCommandTurn(ctx context.Context, sessionID, label string) (release func(), err error) {
	return r.claimTurnSlot(ctx, sessionID, turnqueue.KindCommand, label)
}

// claimSessionTurnSlotCtx claims an autonomous turn slot whose cancel func is
// registered BEFORE it queues (a spawn: see launchSpawn). Without it a stop issued
// while the turn waits for the slot would be silently outlived — the queued turn
// would start after the stop and run to completion.
func (r *Runtime) claimSessionTurnSlotCtx(ctx context.Context, sessionID string, kind turnqueue.Kind, label string) (release func(), err error) {
	return r.claimTurnSlot(ctx, sessionID, kind, label)
}

// claimTurnSlot is the single choke point onto the queue.
func (r *Runtime) claimTurnSlot(ctx context.Context, sessionID string, kind turnqueue.Kind, label string) (func(), error) {
	if err := r.refuseWakeOnArchivedSession(ctx, sessionID, kind); err != nil {
		return func() {}, err
	}
	rel, err := r.turns.Acquire(ctx, sessionID, kind, label)
	if err != nil {
		return rel, err
	}
	// Being the one choke point every inbound command passes through also makes
	// this the place to lift an archived session back into the active list — see
	// sessionreactivate.go. Detached from ctx: the slot is already held, so the
	// tiny metadata write must not be skipped just because the caller's run
	// context is about to be cancelled.
	r.reactivateArchivedSession(context.WithoutCancel(ctx), sessionID, kind)
	return rel, nil
}

// sessionTurnBusy reports whether a turn currently holds the session's slot — the
// replacement for the old coordSlot.running flag that callers outside the queue
// used to read.
func (r *Runtime) sessionTurnBusy(sessionID string) bool { return r.turns.Busy(sessionID) }

// publishTurnQueue announces that a session's admission state changed, so the API
// layer can push the live queue view ("gönderiliyor" / "sırada: uyandırma")
// to every open window. Wired as the queue's observer in NewRuntime's caller; the
// event is deliberately thin — the API re-reads the snapshot rather than the bus
// carrying it, so a burst of changes can coalesce into one read.
func (r *Runtime) publishTurnQueue(sessionID string) {
	if sessionID == "" {
		return
	}
	r.publish(events.Event{
		Type:   events.TypeSessionTurnQueue,
		Target: map[string]string{"view": "chat", "sessionId": sessionID},
	})
	// The same transition is the workspace stream's live "running" edge.
	r.emitLiveness(sessionID)
}

// HasBusyTurns reports whether any session currently holds its admission slot.
func (r *Runtime) HasBusyTurns() bool { return r.turns.HasBusy() }
