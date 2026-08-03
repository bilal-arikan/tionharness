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

// activityIntervalKey keys the heartbeat cadence (see startActivityHeartbeat) on a
// turn context, alongside the touch func.
type activityIntervalKey struct{}

// withActivityInterval records how often a heartbeat should touch the watchdog for
// a long, step-less operation. Kept separate from the touch func so the existing
// step-emitter path (activityTouchFrom) is untouched.
func withActivityInterval(ctx context.Context, d time.Duration) context.Context {
	return context.WithValue(ctx, activityIntervalKey{}, d)
}

func activityIntervalFrom(ctx context.Context) time.Duration {
	d, _ := ctx.Value(activityIntervalKey{}).(time.Duration)
	return d
}

// heartbeatInterval picks a cadence comfortably under the idle window so a
// heartbeat lands well before the watchdog would fire. Half the window, floored so
// a tiny (test-sized) idle still yields a positive tick.
func heartbeatInterval(idle time.Duration) time.Duration {
	d := idle / 2
	if d <= 0 {
		d = idle
	}
	return d
}

// startActivityHeartbeat keeps the idle watchdog satisfied while a single
// long-running operation that emits NO incremental step of its own is in flight —
// a non-streaming provider completion, or a one-shot tool execution (a big
// write_file, a multi-minute shell command). The watchdog is otherwise fed only by
// emitted steps, so such an operation looks identical to a hung turn and gets
// reclaimed mid-work (the false "ASILI KALDI" kill).
//
// A truly wedged operation is still bounded by the hard wall-clock ceiling; the
// heartbeat only holds off the FASTER idle window while real work runs. Streaming
// operations (token/thinking/tool_delta) already touch on every chunk and
// deliberately get NO heartbeat, so a stalled stream is still reclaimed on idle.
//
// Returns a stop func that MUST be called once the operation finishes (call it
// explicitly — do not defer it inside a loop, or the goroutines accumulate until
// the function returns). No-op (nil-safe stop) when the turn carries no watchdog
// (idle disabled, or an interactive turn).
func startActivityHeartbeat(ctx context.Context) func() {
	touch := activityTouchFrom(ctx)
	interval := activityIntervalFrom(ctx)
	if touch == nil || interval <= 0 {
		return func() {}
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ctx.Done():
				return
			case <-t.C:
				touch()
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			close(stop)
			<-done
		})
	}
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
	// Cadence for the in-flight heartbeat (startActivityHeartbeat): a step-less but
	// productive operation (non-streaming completion, one-shot big write) touches on
	// this interval so it is not misread as hung. Only meaningful while idle > 0.
	ctx = withActivityInterval(ctx, heartbeatInterval(idle))
	return ctx, func() {
		hardTimer.Stop()
		idleTimer.Stop()
		cancel(context.Canceled)
	}
}
