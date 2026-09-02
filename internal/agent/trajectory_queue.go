package agent

import (
	"log/slog"
	"sync"
	"sync/atomic"
)

// trajectoryQueue serialises the binder's store writes OFF the coordination
// path. An observer hook runs synchronously inside SpawnWorker / the worker's
// notify path, and a trajectory append is a few atomic file writes — enough to
// change the timing the coordinator's tool loop relies on (a worker must be
// cancellable the instant spawn_worker returns). So the hooks only enqueue;
// one lazily started drainer applies the closures in arrival order, which is
// what keeps "spawned" before "reported" for the same worker.
type trajectoryQueue struct {
	mu      sync.Mutex
	items   []func()
	running bool
	// pending counts enqueued-but-not-finished closures so tests can wait for
	// the projection to settle (backgroundWorkPending).
	pending atomic.Int64
}

// enqueueTrajectoryWork schedules fn on the ordered drainer. Never blocks.
func (r *Runtime) enqueueTrajectoryWork(fn func()) {
	q := &r.trajWork
	q.pending.Add(1)
	q.mu.Lock()
	q.items = append(q.items, fn)
	start := !q.running
	if start {
		q.running = true
	}
	q.mu.Unlock()
	if start {
		go r.drainTrajectoryWork()
	}
}

// drainTrajectoryWork runs queued closures until the queue is empty, then
// exits; the next enqueue starts a fresh drainer. A panicking closure is
// logged and skipped so one bad graph never stalls the projection.
func (r *Runtime) drainTrajectoryWork() {
	q := &r.trajWork
	for {
		q.mu.Lock()
		if len(q.items) == 0 {
			q.running = false
			q.mu.Unlock()
			return
		}
		fn := q.items[0]
		q.items[0] = nil
		q.items = q.items[1:]
		q.mu.Unlock()
		func() {
			defer func() {
				if p := recover(); p != nil {
					r.logger.Error("trajectory binder panicked", "panic", slog.AnyValue(p))
				}
				q.pending.Add(-1)
			}()
			fn()
		}()
	}
}

// trajectoryWorkPending reports whether binder writes are still queued.
func (r *Runtime) trajectoryWorkPending() bool { return r.trajWork.pending.Load() > 0 }
