package agent

import (
	"context"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// Trajectory transition hook wiring (Rota F2): the differ itself lives in
// internal/trajectory; the runtime owns the memo lock, the hook and the queue.

// SetTrajectoryTransitionHook registers the subscriber (the automation
// engine). One hook; nil clears.
func (r *Runtime) SetTrajectoryTransitionHook(fn func(context.Context, TrajectoryTransition)) {
	r.trajTransitionMu.Lock()
	r.trajTransitionHook = fn
	r.trajTransitionMu.Unlock()
}

// observeTrajectoryTransitions is called from OnTrajectoryChange: it diffs the
// snapshot and queues one hook call per transition on the trajectory work
// queue (ordered, off the writer's goroutine).
func (r *Runtime) observeTrajectoryTransitions(ev db.TrajectoryChangeEvent) {
	transitions := r.trajTransitions.Diff(ev)
	if len(transitions) == 0 {
		return
	}
	r.trajTransitionMu.RLock()
	fn := r.trajTransitionHook
	r.trajTransitionMu.RUnlock()
	if fn == nil {
		fn = func(context.Context, TrajectoryTransition) {}
	}
	for _, tr := range transitions {
		tr := tr
		if tr.Kind == TrajTransitionEnd {
			// The end-of-run summary (F3) lands before the trajectory_end rules
			// run, so their prompts and the curator see final numbers.
			id := tr.Trajectory.ID
			r.enqueueTrajectoryWork(func() { r.summarizeOnEnd(id) })
		}
		r.enqueueTrajectoryWork(func() { fn(context.Background(), tr) })
		if tr.Kind == TrajTransitionEnd {
			// The optimizer (F4) checks its threshold last; the LLM call itself
			// leaves the queue on its own goroutine.
			slug, trigger := tr.RecipeSlug(), optimizerTriggerRuns
			if tr.Status == db.TrajStatusFailed {
				trigger = optimizerTriggerFail
			}
			r.enqueueTrajectoryWork(func() { r.MaybeOptimizeRecipe(context.Background(), slug, trigger) })
		}
	}
}
