package providers

import (
	"context"
	"time"
)

// idleOutputFloorKey carries a per-call lower bound for a CLI provider's
// stdout-silence watchdog.
type idleOutputFloorKey struct{}

// WithMinIdleOutputTimeout raises the stdout-silence watchdog to at least d for
// every provider call made with the returned context.
//
// The global window (codexStdoutIdleSec, default 90s) is tuned for a turn inside
// the agentic loop, where the CLI keeps emitting tool/step events and a long gap
// really does mean a wedged process. A one-shot fold — /handoff or /compact —
// has no such heartbeat: it ships the whole transcript in a single request and
// then waits for the model's first token, which on a near-full context window can
// legitimately take minutes. Killing that at 90s turns a slow fold into a hard
// failure, so the fold callers raise the floor instead of the global setting.
//
// d <= 0 is a no-op: this only ever raises a window, never lowers one.
func WithMinIdleOutputTimeout(ctx context.Context, d time.Duration) context.Context {
	if d <= 0 {
		return ctx
	}
	return context.WithValue(ctx, idleOutputFloorKey{}, d)
}

// IdleOutputFloor reports the lower bound carried on ctx by
// WithMinIdleOutputTimeout, or 0 when the caller set none.
func IdleOutputFloor(ctx context.Context) time.Duration {
	d, _ := ctx.Value(idleOutputFloorKey{}).(time.Duration)
	return d
}

// resolveIdleOutputWindow combines the globally configured watchdog window with
// any per-call floor on ctx. A globally DISABLED watchdog (<= 0) stays disabled:
// the floor exists to give a slow call more room, not to re-arm a watchdog the
// operator switched off.
func resolveIdleOutputWindow(ctx context.Context, global time.Duration) time.Duration {
	if global <= 0 {
		return global
	}
	if floor := IdleOutputFloor(ctx); floor > global {
		return floor
	}
	return global
}
