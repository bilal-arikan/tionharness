package agent

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ErrTurnHardTimeout / ErrTurnIdleTimeout are the cancellation CAUSES attached by
// withActivityTimeout. Both a watchdog and a viewer pressing "Durdur" surface as
// context.Canceled on ctx.Err(), so without a cause the caller cannot tell a turn
// that RAN OUT OF TIME from one a human stopped — and a truncated turn gets
// reported as a clean "completed" (the SES17 misreport). Read with context.Cause.
var (
	ErrTurnHardTimeout = errors.New("turn hit its wall-clock ceiling")
	ErrTurnIdleTimeout = errors.New("turn emitted no step within the inactivity window")
)

// activityTouchKey keys the idle-watchdog reset func on a turn context.
type activityTouchKey struct{}

// WithActivityTouch attaches an idle-watchdog reset func to ctx. The step emitter
// (SessionStepEmitter) calls it on every step so live activity keeps the turn
// alive; nil-safe consumers ignore it when absent.
func WithActivityTouch(ctx context.Context, touch func()) context.Context {
	if touch == nil {
		return ctx
	}
	return context.WithValue(ctx, activityTouchKey{}, touch)
}

// activityTouchFrom returns the installed idle-watchdog reset func (nil if none).
func activityTouchFrom(ctx context.Context) func() {
	fn, _ := ctx.Value(activityTouchKey{}).(func())
	return fn
}

// withActivityTimeout bounds a background turn by BOTH an absolute wall-clock
// ceiling (hard) AND an inactivity window (idle): the returned ctx is cancelled
// when either the hard deadline passes OR no touch() arrives within idle. The
// touch func is installed on the ctx (WithActivityTouch) so the step emitter can
// reset the idle timer on every step — a long-but-productive turn (streaming tool
// calls) runs up to hard, while a truly hung turn is reclaimed after idle.
//
// idle <= 0 disables the inactivity window (hard ceiling only). The returned stop
// MUST be called (defer it) to release both timers and the context.
func withActivityTimeout(parent context.Context, hard, idle time.Duration) (context.Context, func()) {
	ctx, cancel := context.WithCancelCause(parent)
	hardTimer := time.AfterFunc(hard, func() { cancel(ErrTurnHardTimeout) })

	if idle <= 0 {
		return ctx, func() { hardTimer.Stop(); cancel(context.Canceled) }
	}

	idleTimer := time.AfterFunc(idle, func() { cancel(ErrTurnIdleTimeout) })
	var mu sync.Mutex
	// Reset the idle timer on every step. Reset is an O(1) reschedule and steps
	// arrive at most a few hundred/sec, so the churn is negligible; the mutex just
	// serialises concurrent touches. Once idle has already fired (cancel ran, ctx
	// permanently Done), a late Reset only schedules a harmless no-op cancel that
	// stop() later cleans up — it never un-cancels a finished turn.
	touch := func() {
		mu.Lock()
		idleTimer.Reset(idle)
		mu.Unlock()
	}
	ctx = WithActivityTouch(ctx, touch)
	return ctx, func() {
		hardTimer.Stop()
		idleTimer.Stop()
		cancel(context.Canceled)
	}
}
